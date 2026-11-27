package br.com.encomendo.core.data.catalog

import androidx.sqlite.SQLiteException
import br.com.encomendo.core.database.CatalogDetailDao
import br.com.encomendo.core.database.CatalogDetailEntity
import br.com.encomendo.core.database.CatalogPageDao
import br.com.encomendo.core.database.CatalogPageEntity
import br.com.encomendo.core.domain.catalog.seesCosts
import br.com.encomendo.core.domain.session.SessionRepository
import br.com.encomendo.core.network.ApiPayloadJson
import kotlinx.coroutines.flow.first
import kotlinx.serialization.KSerializer
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlin.time.Clock

/**
 * CatalogCache keeps the catalog reads the user made lately in Room, so the
 * lists and details still open without a signal (milestone 3; mobile/CLAUDE.md,
 * Auth and offline). It guarantees that:
 *
 * - a row is kept for one store, and another store's rows are never read;
 * - it stores what the API sent, with an owner-only field left out when the
 *   API left it out, and never computes a cost or margin;
 * - a row carries costs when its payload has a `cost`, `productionCost` or
 *   `margin` key, whatever role the device thought it had when it saved it, and
 *   a [view] for a role that does not see costs first deletes every such row,
 *   so a demoted owner never reads the costs kept for them;
 * - a broken cache never breaks a read: a row that cannot be read or decoded is
 *   a miss, and one that cannot be written is skipped.
 */
internal class CatalogCache(
    private val pages: CatalogPageDao,
    private val details: CatalogDetailDao,
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
        val view = CacheView(pages, details, clock, tenant.id)
        if (session.role?.seesCosts() != true) view.dropCosts()
        return view
    }
}

/** A value read from the cache and when it was saved. */
internal class Cached<D>(
    val value: D,
    val atEpochMillis: Long,
)

/** The cache of one store; see [CatalogCache]. */
internal class CacheView(
    private val pages: CatalogPageDao,
    private val details: CatalogDetailDao,
    private val clock: Clock,
    private val tenantId: String,
) {
    suspend fun dropCosts() {
        quietly {
            pages.dropCosts(tenantId)
            details.dropCosts(tenantId)
        }
    }

    suspend fun <D> page(
        kind: String,
        query: String,
        cursor: String?,
        serializer: KSerializer<D>,
    ): Cached<D>? =
        quietly {
            pages.page(tenantId, kind, query, cursor.orEmpty())?.let {
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
            val tree = ApiPayloadJson.encodeToJsonElement(serializer, value)
            val now = clock.now().toEpochMilliseconds()
            pages.savePage(
                CatalogPageEntity(tenantId, kind, query, cursor.orEmpty(), tree.hasCosts(), tree.toString(), now),
            )
            pages.trimLists(tenantId, kind, keep = MAX_LISTS)
        }
    }

    suspend fun <D> detail(
        kind: String,
        id: String,
        serializer: KSerializer<D>,
    ): Cached<D>? =
        quietly {
            details.detail(tenantId, kind, id)?.let {
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
            val tree = ApiPayloadJson.encodeToJsonElement(serializer, value)
            val now = clock.now().toEpochMilliseconds()
            details.upsertDetail(CatalogDetailEntity(tenantId, kind, id, tree.hasCosts(), tree.toString(), now))
            details.trimDetails(tenantId, kind, keep = MAX_DETAILS)
        }
    }

    /**
     * Forgets what a write made stale: every cached page of the [listKind] lists (a row's name, price or cover
     * may sit in any of them) and, when given, the detail [id] of [detailKind].
     */
    suspend fun invalidate(
        listKind: String,
        detailKind: String? = null,
        id: String? = null,
    ) {
        quietly {
            pages.deleteKind(tenantId, listKind)
            if (detailKind != null && id != null) details.deleteDetail(tenantId, detailKind, id)
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

/** The keys only the owner is sent (ADR 0025). */
private val COST_KEYS = setOf("cost", "productionCost", "margin")

/** True when any object in the tree has a key in [COST_KEYS], so the payload is the owner's view. */
private fun JsonElement.hasCosts(): Boolean =
    when (this) {
        is JsonObject -> keys.any { it in COST_KEYS } || values.any { it.hasCosts() }
        is JsonArray -> any { it.hasCosts() }
        else -> false
    }

/** How many lists (a filter and all its pages) and how many details of each kind a store keeps. */
private const val MAX_LISTS = 10
private const val MAX_DETAILS = 100
