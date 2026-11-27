package br.com.encomendo.feature.catalogo

import br.com.encomendo.core.domain.catalog.OwnerField
import br.com.encomendo.core.domain.catalog.isShown
import br.com.encomendo.core.domain.failure.Failure
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import kotlin.test.AfterTest
import kotlin.test.BeforeTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

private const val PAGE = 20

/** The Kits tab: what each role sees, paging, "Mostrar arquivados", "No portal" and the offline copy. */
@OptIn(ExperimentalCoroutinesApi::class) // setMain / resetMain
class KitsViewModelTest {
    private val main = StandardTestDispatcher()

    @BeforeTest
    fun useTestMain() = Dispatchers.setMain(main)

    @AfterTest
    fun restoreMain() = Dispatchers.resetMain()

    @Test
    fun theOwnerSeesTheMarginLineBecauseTheApiSentIt() =
        runTest(main) {
            val shop = Shop(this, Who.Owner)
            val bench = shop.seedItem("Banco Scrambler")
            shop.seedKit("Kit Scrambler", listOf(plainRow(bench.id)))
            val list = shop.kits()

            val state = shop.settle { list.uiState.value }

            assertTrue(state.row.margin.isShown)
            assertTrue(state.row.margin.isShown)
        }

    @Test
    fun anAdminIsSentNoMarginSoNoneShows() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val bench = shop.seedItem("Banco Scrambler")
            shop.seedKit("Kit Scrambler", listOf(plainRow(bench.id)))
            val list = shop.kits()

            val state = shop.settle { list.uiState.value }

            assertEquals(OwnerField.Hidden, state.row.margin)
            assertEquals(OwnerField.Hidden, state.row.cost)
            assertTrue(state.canWrite)
        }

    @Test
    fun theRowCarriesTheApisCountPriceTotalAndDays() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val bench = shop.seedItem("Banco Scrambler")
            val saddle = shop.seedItem("Selim") { copy(priceCents = 5_000) }
            shop.seedKit("Kit Scrambler", listOf(plainRow(bench.id, quantity = 2), plainRow(saddle.id)))
            val list = shop.kits()

            val row = shop.settle { list.uiState.value }.row

            assertEquals(2, row.itemCount)
            assertEquals(25_000, row.price.cents)
            assertEquals(25_000, row.singleItemsTotal.cents, "2 x 100,00 + 50,00, as the API sent it")
            assertEquals(7, row.totalDays)
        }

    @Test
    fun aViewerReadsTheKitsButCannotSwitchOneOnTheStorePage() =
        runTest(main) {
            val shop = Shop(this, Who.Viewer)
            val bench = shop.seedItem("Banco Scrambler")
            val kit = shop.seedKit("Kit Scrambler", listOf(plainRow(bench.id)))
            val list = shop.kits()
            val state = shop.settle { list.uiState.value }
            assertFalse(state.canWrite, "no Novo kit, no switch")

            list.onTogglePublished(state.row, true)
            shop.settle { }

            assertFalse(shop.storedKit.published)
            assertEquals(kit.version, shop.storedKit.version)
        }

    @Test
    fun anEmptyListSaysSo() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val list = shop.kits()

            val state = shop.settle { list.uiState.value }

            assertTrue(state.isEmpty, "Nenhum kit ainda.")
        }

    @Test
    fun aListPastOnePageLoadsMore() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val bench = shop.seedItem("Banco Scrambler")
            repeat(PAGE + 3) { shop.seedKit("Kit ${it.toString().padStart(2, '0')}", listOf(plainRow(bench.id))) }
            val list = shop.kits()

            val first = shop.settle { list.uiState.value }
            assertEquals(PAGE, first.kits.size)
            assertTrue(first.hasMore, "Carregar mais")

            list.onLoadMore()
            val all = shop.settle { list.uiState.value }

            assertEquals(PAGE + 3, all.kits.size)
            assertFalse(all.hasMore)
        }

    @Test
    fun archivedKitsShowOnlyWhenAsked() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val bench = shop.seedItem("Banco Scrambler")
            val kit = shop.seedKit("Kit antigo", listOf(plainRow(bench.id)))
            shop.catalog.archiveKit(kit.id, kit.version)
            val list = shop.kits()
            assertTrue(shop.settle { list.uiState.value }.isEmpty)

            list.onIncludeArchivedChange(true)
            val shown = shop.settle { list.uiState.value }

            assertTrue(shown.row.archived, "Arquivado beside the name")
        }

    @Test
    fun theNoPortalSwitchSavesAndKeepsTheNewVersion() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val bench = shop.seedItem("Banco Scrambler")
            shop.seedKit("Kit Scrambler", listOf(plainRow(bench.id)))
            val list = shop.kits()
            val row = shop.settle { list.uiState.value }.row

            list.onTogglePublished(row, true)
            val after = shop.settle { list.uiState.value }

            assertTrue(after.row.published)
            assertEquals(shop.storedKit.version, after.row.version)
            assertNull(after.savingId)
        }

    @Test
    fun aStaleSwitchSaysSomeoneElseSavedFirstAndReadsTheListAgain() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val bench = shop.seedItem("Banco Scrambler")
            val kit = shop.seedKit("Kit Scrambler", listOf(plainRow(bench.id)))
            val list = shop.kits()
            val stale = shop.settle { list.uiState.value }.row
            shop.catalog.setKitPublished(kit.id, true, kit.version)

            list.onTogglePublished(stale, false)
            val after = shop.settle { list.uiState.value }

            assertEquals(CatalogMessage.KitConflict, after.message)
            assertTrue(after.row.published, "the list shows what the server holds now")
        }

    @Test
    fun withNoConnectionTheDevicesCopyShowsWithItsDate() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val bench = shop.seedItem("Banco Scrambler")
            shop.seedKit("Kit Scrambler", listOf(plainRow(bench.id)))
            shop.state.offlineSince = SAVED_AT
            val list = shop.kits()

            val state = shop.settle { list.uiState.value }

            assertEquals(SAVED_AT, state.offlineSince)
            assertEquals("Kit Scrambler", state.row.name)
        }

    @Test
    fun aFailedFirstReadOffersToTryAgain() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            shop.state.failure = Failure.Offline()
            val list = shop.kits()

            val failed = shop.settle { list.uiState.value }
            assertTrue(failed.canRetry)

            shop.state.failure = null
            list.onRetry()
            assertFalse(shop.settle { list.uiState.value }.canRetry)
        }

    @Test
    fun aKitArchivedFromTheEditorShowsItsNoticeAndReadsAgain() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val bench = shop.seedItem("Banco Scrambler")
            val kit = shop.seedKit("Kit Scrambler", listOf(plainRow(bench.id)))
            val list = shop.kits()
            shop.settle { }

            shop.catalog.archiveKit(kit.id, kit.version)
            shop.changes.publish(CatalogChange.KitArchived("Kit Scrambler"))
            val state = shop.settle { list.uiState.value }

            assertEquals(CatalogChange.KitArchived("Kit Scrambler"), state.notice)
            assertTrue(state.isEmpty, "the archived kit left the list")
        }
}
