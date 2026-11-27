package br.com.encomendo.core.database

import androidx.room3.Room
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.test.runTest
import kotlin.test.AfterTest
import kotlin.test.Test
import kotlin.test.assertNull

/** What Sair leaves of the catalog cache: nothing, of any store (ADR 0016, ADR 0026). */
class CatalogCacheClearTest {
    private val database = buildDatabase(Room.inMemoryDatabaseBuilder<EncomendoDatabase>(), Dispatchers.Default)
    private val pages = database.catalogPageDao()
    private val details = database.catalogDetailDao()

    @AfterTest
    fun close() = database.close()

    @Test
    fun deletingAllLeavesNothingOfAnyStore() =
        runTest {
            pages.savePage(
                CatalogPageEntity(ALTA, "items", "", "", costs = true, payload = "{}", cachedAtEpochMillis = 1),
            )
            pages.savePage(
                CatalogPageEntity(OTHER, "items", "", "", costs = false, payload = "{}", cachedAtEpochMillis = 1),
            )
            details.upsertDetail(
                CatalogDetailEntity(ALTA, "item", "1", costs = true, payload = "{}", cachedAtEpochMillis = 1),
            )
            details.upsertDetail(
                CatalogDetailEntity(OTHER, "item", "2", costs = false, payload = "{}", cachedAtEpochMillis = 1),
            )

            pages.deleteAll()
            details.deleteAll()

            assertNull(pages.page(ALTA, "items", "", ""))
            assertNull(pages.page(OTHER, "items", "", ""))
            assertNull(details.detail(ALTA, "item", "1"))
            assertNull(details.detail(OTHER, "item", "2"))
        }

    private companion object {
        const val ALTA = "0190f3c4-0000-7000-8000-000000000001"
        const val OTHER = "0190f3c4-0000-7000-8000-000000000002"
    }
}
