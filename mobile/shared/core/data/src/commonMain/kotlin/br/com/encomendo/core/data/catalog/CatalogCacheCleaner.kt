package br.com.encomendo.core.data.catalog

import androidx.sqlite.SQLiteException
import br.com.encomendo.core.data.session.DeviceDataCleaner

/**
 * CatalogCacheCleaner empties the offline catalog cache of every store when someone leaves the device: the owner's
 * cached costs must not stay readable on a workshop phone that goes to the next person (ADR 0016, ADR 0026). A store
 * switch does not call it. A database that cannot be written does not stop the logout, which already cleared the
 * session.
 */
class CatalogCacheCleaner(
    private val daos: CatalogCacheDaos,
) : DeviceDataCleaner {
    override suspend fun clear() {
        try {
            daos.clear()
        } catch (_: SQLiteException) {
            // The session is gone already; failing the logout now would show the login with an error on top.
        }
    }
}
