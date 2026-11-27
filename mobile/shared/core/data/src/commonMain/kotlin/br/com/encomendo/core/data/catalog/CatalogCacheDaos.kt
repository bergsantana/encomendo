package br.com.encomendo.core.data.catalog

import br.com.encomendo.core.database.CatalogDetailDao
import br.com.encomendo.core.database.CatalogPageDao

/** CatalogCacheDaos are the two DAOs of the offline catalog cache, passed together to the repositories. */
class CatalogCacheDaos(
    /** The cached list pages. */
    val pages: CatalogPageDao,
    /** The cached details. */
    val details: CatalogDetailDao,
) {
    /**
     * Empties the cache of every store, costs included: Sair leaves nothing of the catalog on the phone, which
     * workshops hand from one person to the next (ADR 0016, ADR 0026).
     */
    suspend fun clear() {
        pages.deleteAll()
        details.deleteAll()
    }
}
