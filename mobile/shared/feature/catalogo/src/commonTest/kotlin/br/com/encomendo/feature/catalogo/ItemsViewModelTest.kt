package br.com.encomendo.feature.catalogo

import br.com.encomendo.core.domain.catalog.OwnerField
import br.com.encomendo.core.domain.catalog.isShown
import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.ui.failure.FailureMessage
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import kotlin.test.AfterTest
import kotlin.test.BeforeTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

@OptIn(ExperimentalCoroutinesApi::class) // setMain / resetMain / runCurrent / advanceTimeBy
class ItemsViewModelTest {
    private val main = StandardTestDispatcher()

    @BeforeTest
    fun useTestMain() = Dispatchers.setMain(main)

    @AfterTest
    fun restoreMain() = Dispatchers.resetMain()

    @Test
    fun readsTheFirstPageAndSaysItIsLoadingUntilThen() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            shop.seedItem("Banco Scrambler")
            val items = shop.items()

            assertTrue(items.uiState.value.loading, "Carregando o catálogo…")
            val state = shop.settle { items.uiState.value }

            assertFalse(state.loading)
            assertEquals(listOf("Banco Scrambler"), state.items.map { it.name })
            assertNull(state.offlineSince)
            assertFalse(state.isEmpty)
        }

    @Test
    fun anEmptyCatalogAndASearchWithNoResultsSayDifferentThings() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val items = shop.items()

            assertTrue(shop.settle { items.uiState.value }.isEmpty, "Nenhum item no catálogo ainda.")
            assertFalse(items.uiState.value.isFiltered)

            items.onQueryChange("banco")
            advanceTimeBy(SEARCH_PAUSE)
            runCurrent()

            assertTrue(items.uiState.value.isEmpty)
            assertTrue(items.uiState.value.isFiltered, "Nenhum item encontrado.")
        }

    @Test
    fun aListPastOnePageLoadsMore() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            repeat(PAGE + 5) { shop.seedItem("Item ${it.toString().padStart(2, '0')}") }
            val items = shop.items()

            val first = shop.settle { items.uiState.value }
            assertEquals(PAGE, first.items.size)
            assertTrue(first.hasMore, "Carregar mais")

            items.onLoadMore()
            val all = shop.settle { items.uiState.value }

            assertEquals(PAGE + 5, all.items.size)
            assertFalse(all.hasMore)
            assertFalse(all.loadingMore)
        }

    @Test
    fun searchReadsAgainOncePauseInTyping() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            shop.seedItem("Banco Scrambler")
            shop.seedItem("Guidão Alto")
            val items = shop.items()
            shop.settle { }

            items.onQueryChange("gui")
            items.onQueryChange("guidão")
            advanceTimeBy(SEARCH_PAUSE - 1)
            runCurrent()
            assertEquals(2, items.uiState.value.items.size, "still typing: the list is as it was")
            advanceTimeBy(1)
            runCurrent()

            assertEquals(
                listOf("Guidão Alto"),
                items.uiState.value.items
                    .map { it.name },
            )
        }

    @Test
    fun theCategoryFilterAndArchivedBoxNarrowTheList() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val seats = shop.catalog.createCategory("Bancos")
            shop.seedItem("Banco Scrambler") { copy(categoryId = seats.id) }
            shop.seedItem("Guidão Alto")
            val old = shop.seedItem("Banco Antigo") { copy(categoryId = seats.id) }
            shop.catalog.archiveItem(old.id, old.version)
            val items = shop.items()
            assertEquals(2, shop.settle { items.uiState.value.items.size }, "archived items are hidden")
            assertEquals(
                listOf("Bancos"),
                items.uiState.value.categories
                    .map { it.name },
            )

            items.onCategoryChange(seats.id)
            assertEquals(
                listOf("Banco Scrambler"),
                shop.settle {
                    items.uiState.value.items
                        .map { it.name }
                },
            )

            items.onIncludeArchivedChange(true)
            val shown = shop.settle { items.uiState.value }
            assertEquals(setOf("Banco Antigo", "Banco Scrambler"), shown.items.map { it.name }.toSet())
            assertTrue(shown.items.single { it.name == "Banco Antigo" }.archived, "Arquivado")
            assertEquals("Bancos", shown.categoryName(shown.items.first().categoryId))
        }

    @Test
    fun theOwnerSeesCostAndMarginAnAdminSeesNeither() =
        runTest(main) {
            val owner = Shop(this, Who.Owner)
            owner.seedItem("Banco Scrambler")
            val admin = Shop(this, Who.Admin)
            admin.seedItem("Banco Scrambler")

            val ownerItems = owner.items()
            val ownerState = owner.settle { ownerItems.uiState.value }
            val adminItems = admin.items()
            val adminState = admin.settle { adminItems.uiState.value }

            assertTrue(ownerState.showsCosts, "the API sent them: Custo and Margem show")
            assertTrue(
                ownerState.row
                    .cost.isShown,
            )
            assertEquals(OwnerField.Hidden, adminState.row.cost)
            assertEquals(OwnerField.Hidden, adminState.row.margin)
            assertFalse(adminState.showsCosts, "no cost column for an admin")
        }

    @Test
    fun onlyRolesThatWriteGetTheButtonsAndTheSwitch() =
        runTest(main) {
            val states =
                Who.entries.associateWith { who ->
                    val shop = Shop(this, who)
                    shop.seedItem("Banco Scrambler")
                    val items = shop.items()
                    shop.settle { items.uiState.value.canWrite }
                }

            assertEquals(mapOf(Who.Owner to true, Who.Admin to true, Who.Viewer to false), states)
        }

    @Test
    fun aViewerCannotFlipNoPortal() =
        runTest(main) {
            val shop = Shop(this, Who.Viewer)
            val seeded = shop.seedItem("Banco Scrambler")
            val items = shop.items()
            val row =
                shop.settle {
                    items.uiState.value.items
                        .single()
                }

            items.onTogglePublished(row, true)
            shop.settle { }

            assertFalse(
                shop.state.items
                    .single { it.id == seeded.id }
                    .published,
            )
        }

    @Test
    fun noPortalShowsTheItemOnTheStorePage() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            shop.seedItem("Banco Scrambler")
            val items = shop.items()
            val row =
                shop.settle {
                    items.uiState.value.items
                        .single()
                }

            items.onTogglePublished(row, true)
            val state = shop.settle { items.uiState.value }

            assertTrue(state.row.published)
            assertEquals(row.version + 1, state.row.version, "the next change uses the new version")
            assertNull(state.savingId)
        }

    @Test
    fun anArchivedItemsSwitchDoesNothing() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val old = shop.seedItem("Banco Antigo")
            shop.catalog.archiveItem(old.id, old.version)
            val items = shop.items()
            items.onIncludeArchivedChange(true)
            val row =
                shop.settle {
                    items.uiState.value.items
                        .single()
                }

            items.onTogglePublished(row, true)
            shop.settle { }

            assertFalse(
                shop.storedItem
                    .published,
            )
        }

    @Test
    fun aSwitchSomeoneElseMovedUnderUsSaysSoAndReadsAgain() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val seeded = shop.seedItem("Banco Scrambler")
            val items = shop.items()
            val row =
                shop.settle {
                    items.uiState.value.items
                        .single()
                }
            shop.catalog.setItemPublished(seeded.id, published = true, version = seeded.version)

            items.onTogglePublished(row, true)
            val state = shop.settle { items.uiState.value }

            assertEquals(CatalogMessage.Conflict, state.message, "Outra pessoa alterou este item…")
            assertEquals(row.version + 1, state.row.version, "the list was read again")
        }

    @Test
    fun theDevicesCopyIsShownWithItsDateWhenOffline() =
        runTest(main) {
            val shop = Shop(this, Who.Viewer)
            shop.seedItem("Banco Scrambler")
            shop.state.offlineSince = SAVED_AT
            val items = shop.items()

            val state = shop.settle { items.uiState.value }

            assertEquals(SAVED_AT, state.offlineSince, "the banner shows over the cached rows")
            assertEquals(listOf("Banco Scrambler"), state.items.map { it.name })
            assertNull(state.message)
        }

    @Test
    fun aFailedFirstReadOffersRetryAndRecovers() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            shop.seedItem("Banco Scrambler")
            shop.state.failure = Failure.Offline()
            val items = shop.items()

            val failed = shop.settle { items.uiState.value }
            assertEquals(CatalogMessage.General(FailureMessage.Offline), failed.message)
            assertTrue(failed.canRetry)

            shop.state.failure = null
            items.onRetry()
            val state = shop.settle { items.uiState.value }

            assertNull(state.message)
            assertEquals(1, state.items.size)
        }

    @Test
    fun aNextPageStillLoadingIsDroppedWhenTheFilterChanges() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            repeat(PAGE + 1) { shop.seedItem("Item ${it.toString().padStart(2, '0')}") }
            val items = shop.items()
            shop.settle { }

            items.onLoadMore()
            items.onIncludeArchivedChange(true)
            val state = shop.settle { items.uiState.value }

            assertEquals(PAGE, state.items.size, "the first page of the new filter, not mixed with the old next page")
            assertFalse(state.loadingMore)
        }

    @Test
    fun whatTheFormChangedShowsAsANoticeAndReadsAgain() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val items = shop.items()
            shop.settle { }
            shop.seedItem("Banco Novo")

            shop.changes.publish(CatalogChange.ItemArchived("Banco Antigo"))
            val state = shop.settle { items.uiState.value }

            assertEquals(CatalogChange.ItemArchived("Banco Antigo"), state.notice)
            assertEquals(listOf("Banco Novo"), state.items.map { it.name })
        }

    @Test
    fun aDeletedCategoryLeavesTheFilter() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val seats = shop.catalog.createCategory("Bancos")
            val items = shop.items()
            shop.settle { }
            items.onCategoryChange(seats.id)
            shop.settle { }
            shop.catalog.deleteCategory(seats.id)

            shop.changes.publish(CatalogChange.Taxonomy)
            val state = shop.settle { items.uiState.value }

            assertNull(state.categoryId)
            assertTrue(state.categories.isEmpty())
        }

    private companion object {
        const val PAGE = 20
        const val SEARCH_PAUSE = 300L
    }
}
