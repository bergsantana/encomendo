package br.com.encomendo.core.database

import androidx.room3.Room
import androidx.sqlite.driver.bundled.BundledSQLiteDriver
import androidx.sqlite.execSQL
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.test.runTest
import java.io.File
import kotlin.test.AfterTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertNull

/**
 * Opens a database file written as schema version 1 (`schemas/.../1.json`, what the app shipped before the
 * catalog cache) with today's database, so Room runs the migration and then checks the result against the
 * current schema: a migration that left a table or column out fails the open.
 */
class MigrationTest {
    private val file = File.createTempFile("encomendo-v1", ".db")

    @AfterTest
    fun delete() {
        file.delete()
    }

    private fun writeVersion1() {
        val connection = BundledSQLiteDriver().open(file.absolutePath)
        try {
            connection.execSQL(
                "CREATE TABLE IF NOT EXISTS `sync_state` (`tenantId` TEXT NOT NULL, `collection` TEXT NOT NULL, " +
                    "`refreshedAtEpochMillis` INTEGER NOT NULL, PRIMARY KEY(`tenantId`, `collection`))",
            )
            connection.execSQL(MASTER_TABLE)
            connection.execSQL(VERSION_1_HASH)
            connection.execSQL("INSERT INTO sync_state VALUES ('store-1', 'orders', $REFRESHED_AT)")
            connection.execSQL("PRAGMA user_version = 1")
        } finally {
            connection.close()
        }
    }

    @Test
    fun version1OpensAtVersion2KeepingItsDataAndGainingTheCatalogCache() =
        runTest {
            writeVersion1()

            val database =
                buildDatabase(Room.databaseBuilder<EncomendoDatabase>(name = file.absolutePath), Dispatchers.Default)
            try {
                // What the app had before is still there.
                assertEquals(REFRESHED_AT, database.syncStateDao().get("store-1", "orders")?.refreshedAtEpochMillis)

                // And the new tables work.
                val cache = database.catalogCacheDao()
                assertNull(cache.page("store-1", false, "items", "", ""))
                cache.savePage(CatalogPageEntity("store-1", false, "items", "", "", """{"items":[]}""", REFRESHED_AT))
                assertNotNull(cache.page("store-1", false, "items", "", ""))
            } finally {
                database.close()
            }
        }

    private companion object {
        const val REFRESHED_AT = 1234L

        const val MASTER_TABLE =
            "CREATE TABLE IF NOT EXISTS room_master_table (id INTEGER PRIMARY KEY,identity_hash TEXT)"

        // The identity hash of schemas/.../1.json.
        const val VERSION_1_HASH =
            "INSERT OR REPLACE INTO room_master_table (id,identity_hash) " +
                "VALUES(42, 'd0ba2369b30e502469940109e18d2c3b')"
    }
}
