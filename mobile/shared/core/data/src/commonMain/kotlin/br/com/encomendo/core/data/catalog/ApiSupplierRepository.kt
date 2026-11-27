package br.com.encomendo.core.data.catalog

import br.com.encomendo.core.data.failure.StoreCalls
import br.com.encomendo.core.domain.catalog.Fetched
import br.com.encomendo.core.domain.paging.Page
import br.com.encomendo.core.domain.session.SessionRepository
import br.com.encomendo.core.domain.supplier.Supplier
import br.com.encomendo.core.domain.supplier.SupplierDraft
import br.com.encomendo.core.domain.supplier.SupplierRepository
import br.com.encomendo.core.network.catalog.SupplierDto
import br.com.encomendo.core.network.catalog.SupplierPageDto
import br.com.encomendo.core.network.catalog.SuppliersApi
import kotlin.time.Clock

/**
 * ApiSupplierRepository reads and writes the store's suppliers through [api]
 * in the session's store. Reads fall back to the offline cache; a write that
 * cannot reach the server is a failure.
 */
class ApiSupplierRepository internal constructor(
    private val api: SuppliersApi,
    private val reads: CatalogReads,
) : SupplierRepository {
    /** Builds the repository over the offline cache kept in [cache] for the store [sessions] says is selected. */
    constructor(
        api: SuppliersApi,
        calls: StoreCalls,
        cache: CatalogCacheDaos,
        sessions: SessionRepository,
        clock: Clock = Clock.System,
    ) : this(api, CatalogReads(calls, CatalogCache(cache.pages, cache.details, sessions, clock)))

    override suspend fun page(
        query: String?,
        cursor: String?,
        limit: Int,
    ): Fetched<Page<Supplier>> =
        reads.list(
            kind = "suppliers",
            query = query.orEmpty(),
            cursor = cursor,
            serializer = SupplierPageDto.serializer(),
            fetch = { api.list(query, cursor, limit) },
            map = { page -> Page(page.items.map(SupplierDto::toSupplier), page.nextCursor) },
        )

    override suspend fun supplier(id: String): Fetched<Supplier> =
        reads.detail(SUPPLIER, id, SupplierDto.serializer(), { api.get(id) }, SupplierDto::toSupplier)

    override suspend fun create(draft: SupplierDraft): Supplier = saved { api.create(draft.toWrite(version = null)) }

    override suspend fun replace(
        id: String,
        version: Int,
        draft: SupplierDraft,
    ): Supplier = saved { api.replace(id, draft.toWrite(version)) }

    private suspend fun saved(call: suspend () -> SupplierDto): Supplier =
        reads.write("suppliers", SUPPLIER, SupplierDto.serializer(), SupplierDto::id, call, SupplierDto::toSupplier)

    private companion object {
        const val SUPPLIER = "supplier"
    }
}
