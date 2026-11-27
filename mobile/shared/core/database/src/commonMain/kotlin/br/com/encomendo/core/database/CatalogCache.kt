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
 * never gains a cost column the app could fill for the wrong role. Rows are
 * kept per store ([tenantId]) and per [costs] view, so a role that does not
 * see costs never reads a page saved for the owner.
 */
@Entity(tableName = "catalog_pages", primaryKeys = ["tenantId", "costs", "kind", "query", "cursor"])
data class CatalogPageEntity(
    val tenantId: String,
    /** True when the page was read by a role that sees costs (the owner). */
    val costs: Boolean,
    /** What the list holds: `items`, `kits`, `categories`, `attributes` or `suppliers`. */
    val kind: String,
    /** The filters of the list, as one string; empty for none. */
    val query: String,
    /** The cursor the page was read with; empty for the first page. */
    val cursor: String,
    /** The page as the API sent it. */
    val payload: String,
    val cachedAtEpochMillis: Long,
)

/** One cached detail (an item, kit or supplier) exactly as the API sent it; see [CatalogPageEntity]. */
@Entity(tableName = "catalog_details", primaryKeys = ["tenantId", "costs", "kind", "id"])
data class CatalogDetailEntity(
    val tenantId: String,
    /** True when the detail was read by a role that sees costs (the owner). */
    val costs: Boolean,
    /** `item`, `kit` or `supplier`. */
    val kind: String,
    val id: String,
    /** The detail as the API sent it. */
    val payload: String,
    val cachedAtEpochMillis: Long,
)

/** Reads and writes the cached catalog reads, always for one store and one costs view. */
@Dao
abstract class CatalogCacheDao {
    /** Returns the cached page, or null when that list page was never read. */
    @Query(
        "SELECT * FROM catalog_pages WHERE tenantId = :tenantId AND costs = :costs " +
            "AND kind = :kind AND query = :query AND cursor = :cursor",
    )
    abstract suspend fun page(
        tenantId: String,
        costs: Boolean,
        kind: String,
        query: String,
        cursor: String,
    ): CatalogPageEntity?

    /** Returns the cached detail, or null when it was never read. */
    @Query(
        "SELECT * FROM catalog_details WHERE tenantId = :tenantId AND costs = :costs AND kind = :kind AND id = :id",
    )
    abstract suspend fun detail(
        tenantId: String,
        costs: Boolean,
        kind: String,
        id: String,
    ): CatalogDetailEntity?

    @Upsert
    protected abstract suspend fun upsertPage(page: CatalogPageEntity)

    @Upsert
    abstract suspend fun upsertDetail(detail: CatalogDetailEntity)

    @Query(
        "DELETE FROM catalog_pages WHERE tenantId = :tenantId AND costs = :costs AND kind = :kind AND query = :query",
    )
    protected abstract suspend fun deletePagesOfList(
        tenantId: String,
        costs: Boolean,
        kind: String,
        query: String,
    )

    /**
     * Keeps only the [keep] most recently read lists of [kind] (a list is one filter, with all its pages), so the
     * cache stays the size of what the user opened lately.
     */
    @Query(
        "DELETE FROM catalog_pages WHERE tenantId = :tenantId AND costs = :costs AND kind = :kind AND query NOT IN (" +
            "SELECT query FROM catalog_pages WHERE tenantId = :tenantId AND costs = :costs AND kind = :kind " +
            "GROUP BY query ORDER BY MAX(cachedAtEpochMillis) DESC LIMIT :keep)",
    )
    abstract suspend fun trimLists(
        tenantId: String,
        costs: Boolean,
        kind: String,
        keep: Int,
    )

    /** Keeps only the [keep] most recently read details of [kind]. */
    @Query(
        "DELETE FROM catalog_details WHERE tenantId = :tenantId AND costs = :costs AND kind = :kind AND id NOT IN (" +
            "SELECT id FROM catalog_details WHERE tenantId = :tenantId AND costs = :costs AND kind = :kind " +
            "ORDER BY cachedAtEpochMillis DESC LIMIT :keep)",
    )
    abstract suspend fun trimDetails(
        tenantId: String,
        costs: Boolean,
        kind: String,
        keep: Int,
    )

    /**
     * Saves a page. A first page (empty cursor) replaces every page of its list, because the later cursors
     * belonged to the old first page and may no longer lead anywhere.
     */
    @Transaction
    open suspend fun savePage(page: CatalogPageEntity) {
        if (page.cursor.isEmpty()) deletePagesOfList(page.tenantId, page.costs, page.kind, page.query)
        upsertPage(page)
    }

    /**
     * Drops what [tenantId] cached for the other costs view. Called before every read and write, so a role change
     * (an owner demoted to admin) never leaves costs readable on the device.
     */
    @Query("DELETE FROM catalog_pages WHERE tenantId = :tenantId AND costs != :costs")
    abstract suspend fun dropOtherViewPages(
        tenantId: String,
        costs: Boolean,
    )

    /** Drops the details cached for the other costs view; see [dropOtherViewPages]. */
    @Query("DELETE FROM catalog_details WHERE tenantId = :tenantId AND costs != :costs")
    abstract suspend fun dropOtherViewDetails(
        tenantId: String,
        costs: Boolean,
    )
}
