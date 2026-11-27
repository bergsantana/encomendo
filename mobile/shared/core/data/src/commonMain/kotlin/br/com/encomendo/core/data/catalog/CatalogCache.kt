package br.com.encomendo.core.data.catalog

import androidx.sqlite.SQLiteException
import br.com.encomendo.core.database.CatalogCacheDao
import br.com.encomendo.core.database.CatalogDetailEntity
import br.com.encomendo.core.database.CatalogPageEntity
import br.com.encomendo.core.domain.catalog.seesCosts
import br.com.encomendo.core.domain.session.SessionRepository
import br.com.encomendo.core.network.ApiPayloadJson
import kotlinx.coroutines.flow.first
import kotlinx.serialization.KSerializer
import kotlin.time.Clock

/**
 * CatalogCache keeps the catalog reads the user made lately in Room, so the
 * lists and details still open without a signal (milestone 3; mobile/CLAUDE.md,
 * Auth and offline). It guarantees that:
 *
 * - a row is kept for one store and one costs view (the owner's, or every other
 *   role's), so another store's rows, and the owner's costs once the role
 *   changed, are never read: a [view] drops what belongs to the other view;
 * - it stores what the API sent, with an owner-only field left out when the
 *   API left it out, and never computes a cost or margin;
 * - a broken cache never breaks a read: a row that cannot be read or decoded is
 *   a miss, and one that cannot be written is skipped.
 */
internal class CatalogCache(
    private val dao: CatalogCacheDao,
    private val sessions: SessionRepository,
    private val clock: Clock,
) {
    /**
     * Returns the cache of the selected store for the signed-in role, or null
     * when no store is selected, where nothing is cached.
     */
    suspend fun view(): CacheView? {
        val session = sessions.session.first()
        val tenant = session?.tenant ?: return null
        val view = CacheView(dao, clock, tenant.id, costs = session.role?.seesCosts() == true)
        view.dropTheOtherView()
        return view
    }
}

/** A value read from the cache and when it was saved. */
internal class Cached<D>(
    val value: D,
    val atEpochMillis: Long,
)

/** The cache of one store for one costs view; see [CatalogCache]. */
internal class CacheView(
    private val dao: CatalogCacheDao,
    private val clock: Clock,
    private val tenantId: String,
    private val costs: Boolean,
) {
    suspend fun dropTheOtherView() {
        quietly {
            dao.dropOtherViewPages(tenantId, costs)
            dao.dropOtherViewDetails(tenantId, costs)
        }
    }

    suspend fun <D> page(
        kind: String,
        query: String,
        cursor: String?,
        serializer: KSerializer<D>,
    ): Cached<D>? =
        quietly {
            dao.page(tenantId, costs, kind, query, cursor.orEmpty())?.let {
                Cached(ApiPayloadJson.decodeFromString(serializer, it.payload), it.cachedAtEpochMillis)
            }
        }

    suspend fun <D> savePage(
        kind: String,
        query: String,
        cursor: String?,
        serializer: KSerializer<D>,
        value: D,
    ) {
        quietly {
            val payload = ApiPayloadJson.encodeToString(serializer, value)
            val now = clock.now().toEpochMilliseconds()
            dao.savePage(CatalogPageEntity(tenantId, costs, kind, query, cursor.orEmpty(), payload, now))
            dao.trimLists(tenantId, costs, kind, keep = MAX_LISTS)
        }
    }

    suspend fun <D> detail(
        kind: String,
        id: String,
        serializer: KSerializer<D>,
    ): Cached<D>? =
        quietly {
            dao.detail(tenantId, costs, kind, id)?.let {
                Cached(ApiPayloadJson.decodeFromString(serializer, it.payload), it.cachedAtEpochMillis)
            }
        }

    suspend fun <D> saveDetail(
        kind: String,
        id: String,
        serializer: KSerializer<D>,
        value: D,
    ) {
        quietly {
            val payload = ApiPayloadJson.encodeToString(serializer, value)
            dao.upsertDetail(
                CatalogDetailEntity(tenantId, costs, kind, id, payload, clock.now().toEpochMilliseconds()),
            )
            dao.trimDetails(tenantId, costs, kind, keep = MAX_DETAILS)
        }
    }

    // A cache is a convenience: a database or decoding fault means "not cached", never a failed read.
    private suspend fun <T> quietly(block: suspend () -> T): T? =
        try {
            block()
        } catch (_: SQLiteException) {
            null
        } catch (_: IllegalArgumentException) {
            // SerializationException is an IllegalArgumentException: the saved JSON is not what we read now.
            null
        }
}

/** How many lists (a filter and all its pages) and how many details of each kind a store keeps. */
private const val MAX_LISTS = 10
private const val MAX_DETAILS = 100
