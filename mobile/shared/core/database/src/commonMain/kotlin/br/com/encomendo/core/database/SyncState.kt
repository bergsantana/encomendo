package br.com.encomendo.core.database

import androidx.room3.Dao
import androidx.room3.Entity
import androidx.room3.Query
import androidx.room3.Upsert

/**
 * When a cached collection (for example "orders" or "production_board") was
 * last refreshed from the server, per tenant. Screens use it to tell users how
 * fresh the offline copy is.
 */
@Entity(tableName = "sync_state", primaryKeys = ["tenantId", "collection"])
data class SyncStateEntity(
    val tenantId: String,
    val collection: String,
    val refreshedAtEpochMillis: Long,
)

/** Reads and writes [SyncStateEntity] rows. */
@Dao
interface SyncStateDao {
    /** Returns the sync state of one collection, or null if it was never refreshed. */
    @Query("SELECT * FROM sync_state WHERE tenantId = :tenantId AND collection = :collection")
    suspend fun get(
        tenantId: String,
        collection: String,
    ): SyncStateEntity?

    /** Inserts or replaces the sync state of one collection. */
    @Upsert
    suspend fun upsert(state: SyncStateEntity)

    /** Removes every row of a tenant, used when the user switches tenant or logs out. */
    @Query("DELETE FROM sync_state WHERE tenantId = :tenantId")
    suspend fun clearTenant(tenantId: String)
}
