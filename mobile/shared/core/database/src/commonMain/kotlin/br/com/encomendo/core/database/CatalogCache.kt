package br.com.encomendo.core.database

import androidx.room3.Dao
import androidx.room3.Entity
import androidx.room3.Query
import androidx.room3.Transaction
import androidx.room3.Upsert

/**
 * One cached page of a catalog list (items, kits, categories, fields,
 * suppliers), exactly as the API sent it in [payload].
 *
 * The payload is the API's own JSON, so an owner-only cost the API left out
 * stays left out and one it sent as null stays null (ADR 0025); the table
 * never gains a cost column the app could fill for the wrong role. [costs]
 * says the payload carries a cost, production cost or margin key, read from
 * the payload itself, so a role that does not see costs never reads the row.
 * Rows are kept per store ([tenantId]).
 */
@Entity(tableName = "catalog_pages", primaryKeys = ["tenantId", "kind", "query", "cursor"])
data class CatalogPageEntity(
    val tenantId: String,
    /** What the list holds: `items`, `kits`, `categories`, `attributes` or `suppliers`. */
    val kind: String,
    /** The filters of the list, as one string; empty for none. */
    val query: String,
    /** The cursor the page was read with; empty for the first page. */
    val cursor: String,
    /** True when the payload has a cost, production cost or margin key (the owner's view). */
    val costs: Boolean,
    /** The page as the API sent it. */
    val payload: String,
    val cachedAtEpochMillis: Long,
)

/** One cached detail (an item, kit or supplier) exactly as the API sent it; see [CatalogPageEntity]. */
@Entity(tableName = "catalog_details", primaryKeys = ["tenantId", "kind", "id"])
data class CatalogDetailEntity(
    val tenantId: String,
    /** `item`, `kit` or `supplier`. */
    val kind: String,
    val id: String,
    /** True when the payload has a cost, production cost or margin key (the owner's view). */
    val costs: Boolean,
    /** The detail as the API sent it. */
    val payload: String,
    val cachedAtEpochMillis: Long,
)

/** Reads and writes the cached catalog list pages of one store. */
@Dao
abstract class CatalogPageDao {
    /** Returns the cached page, or null when that list page was never read. */
    @Query(
        "SELECT * FROM catalog_pages WHERE tenantId = :tenantId AND kind = :kind " +
            "AND query = :query AND cursor = :cursor",
    )
    abstract suspend fun page(
        tenantId: String,
        kind: String,
        query: String,
        cursor: String,
    ): CatalogPageEntity?

    @Upsert
    protected abstract suspend fun upsertPage(page: CatalogPageEntity)

    @Query("DELETE FROM catalog_pages WHERE tenantId = :tenantId AND kind = :kind AND query = :query")
    protected abstract suspend fun deleteList(
        tenantId: String,
        kind: String,
        query: String,
    )

    /**
     * Saves a page. A first page (empty cursor) replaces every page of its list, because the later cursors
     * belonged to the old first page and may no longer lead anywhere.
     */
    @Transaction
    open suspend fun savePage(page: CatalogPageEntity) {
        if (page.cursor.isEmpty()) deleteList(page.tenantId, page.kind, page.query)
        upsertPage(page)
    }

    /**
     * Keeps only the [keep] most recently read lists of [kind] (a list is one filter, with all its pages), so the
     * cache stays the size of what the user opened lately.
     */
    @Query(
        "DELETE FROM catalog_pages WHERE tenantId = :tenantId AND kind = :kind AND query NOT IN (" +
            "SELECT query FROM catalog_pages WHERE tenantId = :tenantId AND kind = :kind " +
            "GROUP BY query ORDER BY MAX(cachedAtEpochMillis) DESC LIMIT :keep)",
    )
    abstract suspend fun trimLists(
        tenantId: String,
        kind: String,
        keep: Int,
    )

    /** Drops every page of [kind], after a write made them stale. */
    @Query("DELETE FROM catalog_pages WHERE tenantId = :tenantId AND kind = :kind")
    abstract suspend fun deleteKind(
        tenantId: String,
        kind: String,
    )

    /**
     * Drops the pages that carry costs. Called when the signed-in role does not see costs, so a demoted owner
     * never leaves costs readable on the device.
     */
    @Query("DELETE FROM catalog_pages WHERE tenantId = :tenantId AND costs = 1")
    abstract suspend fun dropCosts(tenantId: String)

    /** Drops every page of every store, when someone leaves the device (ADR 0016, ADR 0026). */
    @Query("DELETE FROM catalog_pages")
    abstract suspend fun deleteAll()
}

/** Reads and writes the cached catalog details of one store. */
@Dao
abstract class CatalogDetailDao {
    /** Returns the cached detail, or null when it was never read. */
    @Query("SELECT * FROM catalog_details WHERE tenantId = :tenantId AND kind = :kind AND id = :id")
    abstract suspend fun detail(
        tenantId: String,
        kind: String,
        id: String,
    ): CatalogDetailEntity?

    @Upsert
    abstract suspend fun upsertDetail(detail: CatalogDetailEntity)

    /** Keeps only the [keep] most recently read details of [kind]. */
    @Query(
        "DELETE FROM catalog_details WHERE tenantId = :tenantId AND kind = :kind AND id NOT IN (" +
            "SELECT id FROM catalog_details WHERE tenantId = :tenantId AND kind = :kind " +
            "ORDER BY cachedAtEpochMillis DESC LIMIT :keep)",
    )
    abstract suspend fun trimDetails(
        tenantId: String,
        kind: String,
        keep: Int,
    )

    /** Drops one detail, after a write made it stale. */
    @Query("DELETE FROM catalog_details WHERE tenantId = :tenantId AND kind = :kind AND id = :id")
    abstract suspend fun deleteDetail(
        tenantId: String,
        kind: String,
        id: String,
    )

    /** Drops the details that carry costs; see [CatalogPageDao.dropCosts]. */
    @Query("DELETE FROM catalog_details WHERE tenantId = :tenantId AND costs = 1")
    abstract suspend fun dropCosts(tenantId: String)

    /** Drops every detail of every store; see [CatalogPageDao.deleteAll]. */
    @Query("DELETE FROM catalog_details")
    abstract suspend fun deleteAll()
}
