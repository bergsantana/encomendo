package br.com.encomendo.feature.catalogo

import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.ui.failure.FailureMessage
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

/** Categorias. */
@OptIn(ExperimentalCoroutinesApi::class) // setMain / resetMain
class CategoriesViewModelTest {
    private val main = StandardTestDispatcher()

    @BeforeTest
    fun useTestMain() = Dispatchers.setMain(main)

    @AfterTest
    fun restoreMain() = Dispatchers.resetMain()

    @Test
    fun categoriesListInTheStoresOrder() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            shop.catalog.createCategory("Bancos")
            shop.catalog.createCategory("Guidões")

            val created = shop.categories()
            val state = shop.settle { created.uiState.value }

            assertEquals(listOf("Bancos", "Guidões"), state.categories.map { it.name })
            assertFalse(state.loading)
            assertTrue(state.canWrite)
        }

    @Test
    fun anAdminAddsACategoryAndTheListLearnsOfIt() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val categories = shop.categories()
            shop.settle { }

            categories.onNewNameChange("  Bancos ")
            categories.onAdd()
            val state = shop.settle { categories.uiState.value }

            assertEquals(listOf("Bancos"), state.categories.map { it.name })
            assertEquals("", state.newName)
            assertEquals(listOf<CatalogChange>(CatalogChange.Taxonomy), shop.published)
        }

    @Test
    fun anEmptyCategoryNameIsMarkedAndAUsedOneIsTaken() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            shop.catalog.createCategory("Bancos")
            val categories = shop.categories()
            shop.settle { }

            categories.onAdd()
            assertEquals(NameProblem.Missing, categories.uiState.value.newProblem, "Informe o nome da categoria.")
            categories.onNewNameChange("bancos")
            assertNull(categories.uiState.value.newProblem)
            categories.onAdd()
            val state = shop.settle { categories.uiState.value }

            assertEquals(NameProblem.Taken, state.newProblem, "Já existe uma categoria com este nome.")
            assertNull(state.message)
            assertEquals(1, state.categories.size)
        }

    @Test
    fun aCategoryIsRenamedAndMoved() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            shop.catalog.createCategory("Bancos")
            val second = shop.catalog.createCategory("Guidões")
            val categories = shop.categories()
            shop.settle { }

            categories.onStartRename(second)
            categories.onRenameChange("Guidões e manoplas")
            categories.onSaveRename()
            shop.settle { }
            categories.onMove(categories.uiState.value.categories[1], up = true)
            val state = shop.settle { categories.uiState.value }

            assertEquals(listOf("Guidões e manoplas", "Bancos"), state.categories.map { it.name })
            assertNull(state.edit)
        }

    @Test
    fun aRenameToAnotherCategorysNameIsMarkedAndStaysOpen() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val first = shop.catalog.createCategory("Bancos")
            shop.catalog.createCategory("Guidões")
            val categories = shop.categories()
            shop.settle { }

            categories.onStartRename(first)
            categories.onRenameChange("Guidões")
            categories.onSaveRename()
            val state = shop.settle { categories.uiState.value }

            assertEquals(NameProblem.Taken, state.edit?.problem)
            assertEquals(listOf("Bancos", "Guidões"), state.categories.map { it.name })
        }

    @Test
    fun aCategoryWithItemsIsNotDeletedAndItsRowSaysWhy() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val bancos = shop.catalog.createCategory("Bancos")
            val guidoes = shop.catalog.createCategory("Guidões")
            shop.seedItem("Banco Scrambler") { copy(categoryId = bancos.id) }
            val categories = shop.categories()
            shop.settle { }

            categories.onDelete(bancos)
            val refused = shop.settle { categories.uiState.value }
            assertEquals(bancos.id, refused.inUseId, "Esta categoria tem itens…")
            assertEquals(2, refused.categories.size)
            assertNull(refused.message)

            categories.onDelete(guidoes)
            val deleted = shop.settle { categories.uiState.value }
            assertEquals(listOf("Bancos"), deleted.categories.map { it.name })
            assertNull(deleted.inUseId)
        }

    @Test
    fun aViewerSeesCategoriesAndChangesNone() =
        runTest(main) {
            val shop = Shop(this, Who.Viewer)
            val bancos = shop.catalog.createCategory("Bancos")
            val categories = shop.categories()
            shop.settle { }

            categories.onNewNameChange("Guidões")
            categories.onAdd()
            categories.onDelete(bancos)
            shop.settle { }

            assertFalse(categories.uiState.value.canWrite)
            assertEquals(listOf("Bancos"), shop.state.categories.map { it.name })
        }

    @Test
    fun categoriesThatCannotBeReadSayWhy() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            shop.state.failure = Failure.Offline()

            val created = shop.categories()
            val state = shop.settle { created.uiState.value }

            assertEquals(CatalogMessage.General(FailureMessage.Offline), state.message)
        }
}
