package br.com.encomendo.core.data.catalog

import br.com.encomendo.core.database.CatalogDetailDao
import br.com.encomendo.core.database.CatalogPageDao

/** CatalogCacheDaos are the two DAOs of the offline catalog cache, passed together to the repositories. */
class CatalogCacheDaos(
    /** The cached list pages. */
    val pages: CatalogPageDao,
    /** The cached details. */
    val details: CatalogDetailDao,
)
