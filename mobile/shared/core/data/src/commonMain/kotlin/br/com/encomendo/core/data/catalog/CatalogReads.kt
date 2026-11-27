package br.com.encomendo.core.data.catalog

import br.com.encomendo.core.data.failure.StoreCalls
import br.com.encomendo.core.domain.catalog.Fetched
import br.com.encomendo.core.domain.failure.Failure
import kotlinx.serialization.KSerializer

/**
 * CatalogReads is how every catalog and supplier read goes through the store's
 * calls and the offline cache. A read asks the server; when the server cannot
 * be reached ([Failure.Offline]) it answers from the copy saved by the last
 * successful read of the same list or detail, and says when that copy was
 * saved. Any other failure, and a read with no usable copy, throws as it came.
 *
 * Writes use [write] and never fall back: an offline write is a failure, since
 * catalog writes are not queued (ADR 0025). A successful write forgets the
 * cached list pages and the detail it made stale, so an offline read never
 * shows the row as it was before the user's own change.
 */
internal class CatalogReads(
    private val calls: StoreCalls,
    private val cache: CatalogCache,
) {
    /** Reads a list page of [kind] filtered by [query] from [cursor]. */
    suspend fun <D, T> list(
        kind: String,
        query: String,
        cursor: String?,
        serializer: KSerializer<D>,
        fetch: suspend () -> D,
        map: (D) -> T,
    ): Fetched<T> =
        read(
            fetch = fetch,
            map = map,
            load = { it.page(kind, query, cursor, serializer) },
            save = { view, dto -> view.savePage(kind, query, cursor, serializer, dto) },
        )

    /** Reads the detail [id] of [kind]. */
    suspend fun <D, T> detail(
        kind: String,
        id: String,
        serializer: KSerializer<D>,
        fetch: suspend () -> D,
        map: (D) -> T,
    ): Fetched<T> =
        read(
            fetch = fetch,
            map = map,
            load = { it.detail(kind, id, serializer) },
            save = { view, dto -> view.saveDetail(kind, id, serializer, dto) },
        )

    /**
     * Makes a write that returns the saved row of [detailKind] and keeps that row as the cached detail, so it
     * opens offline as saved; every cached page of the [listKind] lists is forgotten. [idOf] reads the id from
     * the answer.
     */
    suspend fun <D, T> write(
        listKind: String,
        detailKind: String,
        serializer: KSerializer<D>,
        idOf: (D) -> String,
        call: suspend () -> D,
        map: (D) -> T,
    ): T {
        val view = cache.view()
        val saved = calls.inStore { call().let { dto -> dto to map(dto) } }
        view?.invalidate(listKind)
        view?.saveDetail(detailKind, idOf(saved.first), serializer, saved.first)
        return saved.second
    }

    /**
     * Makes a write that returns nothing worth keeping, then forgets the cached pages of the [listKind] lists
     * and, when [detailKind] and [id] are given, that cached detail.
     */
    suspend fun <T> write(
        listKind: String,
        detailKind: String? = null,
        id: String? = null,
        call: suspend () -> T,
    ): T {
        val view = cache.view()
        val answer = calls.inStore(call)
        view?.invalidate(listKind, detailKind, id)
        return answer
    }

    /** Runs a step that is not itself one call, such as the photo upload's three, through the store's calls. */
    suspend fun <T> inStore(call: suspend () -> T): T = calls.inStore(call)

    private suspend fun <D, T> read(
        fetch: suspend () -> D,
        map: (D) -> T,
        load: suspend (CacheView) -> Cached<D>?,
        save: suspend (CacheView, D) -> Unit,
    ): Fetched<T> {
        val view = cache.view()
        try {
            val answer = calls.inStore { fetch().let { dto -> dto to map(dto) } }
            view?.let { save(it, answer.first) }
            return Fetched(answer.second)
        } catch (offline: Failure.Offline) {
            val cached = view?.let { load(it) } ?: throw offline
            return fromCopy(cached, map, offline)
        }
    }

    // A copy that no longer maps is no copy: the caller learns what happened (no signal), not what the copy lacks.
    private fun <D, T> fromCopy(
        cached: Cached<D>,
        map: (D) -> T,
        offline: Failure.Offline,
    ): Fetched<T> =
        try {
            Fetched(map(cached.value), cached.atEpochMillis)
        } catch (_: Failure.Unexpected) {
            throw offline
        }
}
