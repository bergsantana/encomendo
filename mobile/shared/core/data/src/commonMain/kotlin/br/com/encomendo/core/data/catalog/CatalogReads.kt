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
 * saved. Any other failure, and a read with no copy, throws as it came.
 *
 * Writes use [write] and never fall back: an offline write is a failure, since
 * catalog writes are not queued (ADR 0025).
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
     * Makes a write in the store and keeps the detail it returned, so the saved row opens offline as saved.
     * [kind] and [serializer] say which detail; [idOf] reads its id from the answer.
     */
    suspend fun <D, T> write(
        kind: String,
        serializer: KSerializer<D>,
        idOf: (D) -> String,
        call: suspend () -> D,
        map: (D) -> T,
    ): T {
        val view = cache.view()
        val saved = calls.inStore { call().let { dto -> dto to map(dto) } }
        view?.saveDetail(kind, idOf(saved.first), serializer, saved.first)
        return saved.second
    }

    /** Makes a write that returns nothing worth keeping. */
    suspend fun <T> write(call: suspend () -> T): T = calls.inStore(call)

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
            return Fetched(map(cached.value), cached.atEpochMillis)
        }
    }
}
