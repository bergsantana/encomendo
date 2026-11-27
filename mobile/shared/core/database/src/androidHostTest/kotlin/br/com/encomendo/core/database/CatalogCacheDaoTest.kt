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
    private val pages = database.catalogPageDao()
    private val details = database.catalogDetailDao()

    @AfterTest
    fun close() = database.close()

    private fun page(
        cursor: String = "",
        query: String = "",
        tenant: String = ALTA,
        costs: Boolean = false,
        at: Long = 1,
        payload: String = """{"items":[]}""",
    ) = CatalogPageEntity(tenant, "items", query, cursor, costs, payload, at)

    private fun detail(
        id: String,
        tenant: String = ALTA,
        costs: Boolean = false,
        at: Long = 1,
    ) = CatalogDetailEntity(tenant, "item", id, costs, """{"id":"$id"}""", at)

    @Test
    fun aSavedPageComesBackAsTheApiSentIt() =
        runTest {
            val payload = """{"items":[{"id":"1"}],"nextCursor":"c2"}"""

            pages.savePage(page(payload = payload, at = SAVED_AT))

            val saved = assertNotNull(pages.page(ALTA, "items", "", ""))
            assertEquals(payload, saved.payload)
            assertEquals(SAVED_AT, saved.cachedAtEpochMillis)
            assertNull(pages.page(ALTA, "items", "", "c2"), "a page never read is not there")
        }

    @Test
    fun aNewFirstPageReplacesTheListsOldPages() =
        runTest {
            pages.savePage(page())
            pages.savePage(page(cursor = "c2"))
            pages.savePage(page(query = "banco"))

            pages.savePage(page(payload = """{"items":[{"id":"new"}]}"""))

            assertNull(pages.page(ALTA, "items", "", "c2"), "the old cursor no longer leads anywhere")
            assertNotNull(pages.page(ALTA, "items", "banco", ""), "another list is untouched")
            assertEquals("""{"items":[{"id":"new"}]}""", pages.page(ALTA, "items", "", "")?.payload)
        }

    @Test
    fun aSecondPageDoesNotReplaceTheFirst() =
        runTest {
            pages.savePage(page())

            pages.savePage(page(cursor = "c2"))

            assertNotNull(pages.page(ALTA, "items", "", ""))
            assertNotNull(pages.page(ALTA, "items", "", "c2"))
        }

    @Test
    fun storesNeverShareRows() =
        runTest {
            pages.savePage(page(tenant = ALTA))
            details.upsertDetail(detail("1", tenant = ALTA))

            assertNull(pages.page(OTHER, "items", "", ""))
            assertNull(details.detail(OTHER, "item", "1"))
        }

    @Test
    fun droppingCostsKeepsTheRowsWithoutThemAndOnlyInThatStore() =
        runTest {
            pages.savePage(page(query = "owner", costs = true))
            pages.savePage(page(query = "plain", costs = false))
            pages.savePage(page(query = "owner", costs = true, tenant = OTHER))
            details.upsertDetail(detail("1", costs = true))
            details.upsertDetail(detail("2", costs = false))

            pages.dropCosts(ALTA)
            details.dropCosts(ALTA)

            assertNull(pages.page(ALTA, "items", "owner", ""))
            assertNotNull(pages.page(ALTA, "items", "plain", ""))
            assertNotNull(pages.page(OTHER, "items", "owner", ""))
            assertNull(details.detail(ALTA, "item", "1"))
            assertNotNull(details.detail(ALTA, "item", "2"))
        }

    @Test
    fun aWriteDropsTheKindsPagesAndOneDetail() =
        runTest {
            pages.savePage(page(query = "a"))
            pages.savePage(page(query = "a", cursor = "c2"))
            pages.savePage(page(tenant = OTHER))
            details.upsertDetail(detail("1"))
            details.upsertDetail(detail("2"))

            pages.deleteKind(ALTA, "items")
            details.deleteDetail(ALTA, "item", "1")

            assertNull(pages.page(ALTA, "items", "a", ""))
            assertNull(pages.page(ALTA, "items", "a", "c2"))
            assertNotNull(pages.page(OTHER, "items", "", ""))
            assertNull(details.detail(ALTA, "item", "1"))
            assertNotNull(details.detail(ALTA, "item", "2"))
        }

    @Test
    fun keepsOnlyTheMostRecentlyReadDetails() =
        runTest {
            for (n in 1..DETAILS_READ) details.upsertDetail(detail("$n", at = n.toLong()))

            details.trimDetails(ALTA, "item", keep = 2)

            assertNull(details.detail(ALTA, "item", "3"))
            assertNotNull(details.detail(ALTA, "item", "4"))
            assertNotNull(details.detail(ALTA, "item", "$DETAILS_READ"))
        }

    @Test
    fun keepsOnlyTheMostRecentlyReadLists() =
        runTest {
            pages.savePage(page(query = "a", at = 1))
            pages.savePage(page(query = "a", cursor = "c2", at = 1))
            pages.savePage(page(query = "b", at = 2))
            pages.savePage(page(query = "c", at = 3))

            pages.trimLists(ALTA, "items", keep = 2)

            assertNull(pages.page(ALTA, "items", "a", ""))
            assertNull(pages.page(ALTA, "items", "a", "c2"), "a list goes with all its pages")
            assertNotNull(pages.page(ALTA, "items", "b", ""))
            assertNotNull(pages.page(ALTA, "items", "c", ""))
        }

    private companion object {
        const val ALTA = "0190f3c4-0000-7000-8000-000000000001"
        const val OTHER = "0190f3c4-0000-7000-8000-000000000002"
        const val SAVED_AT = 42L
        const val DETAILS_READ = 5
    }
}
