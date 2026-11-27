package br.com.encomendo.core.database

import androidx.room3.Room
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.test.runTest
import kotlin.test.AfterTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertNull

class CatalogCacheDaoTest {
    private val database = buildDatabase(Room.inMemoryDatabaseBuilder<EncomendoDatabase>(), Dispatchers.Default)
    private val dao = database.catalogCacheDao()

    @AfterTest
    fun close() = database.close()

    private fun page(
        cursor: String = "",
        query: String = "",
        tenant: String = ALTA,
        costs: Boolean = false,
        at: Long = 1,
        payload: String = """{"items":[]}""",
    ) = CatalogPageEntity(tenant, costs, "items", query, cursor, payload, at)

    private fun detail(
        id: String,
        tenant: String = ALTA,
        costs: Boolean = false,
        at: Long = 1,
    ) = CatalogDetailEntity(tenant, costs, "item", id, """{"id":"$id"}""", at)

    @Test
    fun aSavedPageComesBackAsTheApiSentIt() =
        runTest {
            val payload = """{"items":[{"id":"1"}],"nextCursor":"c2"}"""

            dao.savePage(page(payload = payload, at = SAVED_AT))

            val saved = assertNotNull(dao.page(ALTA, false, "items", "", ""))
            assertEquals(payload, saved.payload)
            assertEquals(SAVED_AT, saved.cachedAtEpochMillis)
            assertNull(dao.page(ALTA, false, "items", "", "c2"), "a page never read is not there")
        }

    @Test
    fun aNewFirstPageReplacesTheListsOldPages() =
        runTest {
            dao.savePage(page())
            dao.savePage(page(cursor = "c2"))
            dao.savePage(page(query = "banco"))

            dao.savePage(page(payload = """{"items":[{"id":"new"}]}"""))

            assertNull(dao.page(ALTA, false, "items", "", "c2"), "the old cursor no longer leads anywhere")
            assertNotNull(dao.page(ALTA, false, "items", "banco", ""), "another list is untouched")
            assertEquals("""{"items":[{"id":"new"}]}""", dao.page(ALTA, false, "items", "", "")?.payload)
        }

    @Test
    fun aSecondPageDoesNotReplaceTheFirst() =
        runTest {
            dao.savePage(page())

            dao.savePage(page(cursor = "c2"))

            assertNotNull(dao.page(ALTA, false, "items", "", ""))
            assertNotNull(dao.page(ALTA, false, "items", "", "c2"))
        }

    @Test
    fun storesNeverShareRows() =
        runTest {
            dao.savePage(page(tenant = ALTA))
            dao.upsertDetail(detail("1", tenant = ALTA))

            assertNull(dao.page(OTHER, false, "items", "", ""))
            assertNull(dao.detail(OTHER, false, "item", "1"))
        }

    @Test
    fun theOwnersViewAndTheOthersAreApart() =
        runTest {
            dao.savePage(page(costs = true))
            dao.upsertDetail(detail("1", costs = true))

            assertNull(dao.page(ALTA, false, "items", "", ""), "an admin never reads the owner's page")
            assertNull(dao.detail(ALTA, false, "item", "1"))
        }

    @Test
    fun aRoleChangeDropsTheOtherViewsRows() =
        runTest {
            dao.savePage(page(costs = true))
            dao.upsertDetail(detail("1", costs = true))
            dao.savePage(page(costs = false, tenant = OTHER))

            // The owner became an admin: what was kept with costs goes, in this store only.
            dao.dropOtherViewPages(ALTA, costs = false)
            dao.dropOtherViewDetails(ALTA, costs = false)

            assertNull(dao.page(ALTA, true, "items", "", ""))
            assertNull(dao.detail(ALTA, true, "item", "1"))
            assertNotNull(dao.page(OTHER, false, "items", "", ""))
        }

    @Test
    fun keepsOnlyTheMostRecentlyReadDetails() =
        runTest {
            for (n in 1..DETAILS_READ) dao.upsertDetail(detail("$n", at = n.toLong()))

            dao.trimDetails(ALTA, false, "item", keep = 2)

            assertNull(dao.detail(ALTA, false, "item", "3"))
            assertNotNull(dao.detail(ALTA, false, "item", "4"))
            assertNotNull(dao.detail(ALTA, false, "item", "$DETAILS_READ"))
        }

    @Test
    fun keepsOnlyTheMostRecentlyReadLists() =
        runTest {
            dao.savePage(page(query = "a", at = 1))
            dao.savePage(page(query = "a", cursor = "c2", at = 1))
            dao.savePage(page(query = "b", at = 2))
            dao.savePage(page(query = "c", at = 3))

            dao.trimLists(ALTA, false, "items", keep = 2)

            assertNull(dao.page(ALTA, false, "items", "a", ""))
            assertNull(dao.page(ALTA, false, "items", "a", "c2"), "a list goes with all its pages")
            assertNotNull(dao.page(ALTA, false, "items", "b", ""))
            assertNotNull(dao.page(ALTA, false, "items", "c", ""))
        }

    private companion object {
        const val ALTA = "0190f3c4-0000-7000-8000-000000000001"
        const val OTHER = "0190f3c4-0000-7000-8000-000000000002"
        const val SAVED_AT = 42L
        const val DETAILS_READ = 5
    }
}
