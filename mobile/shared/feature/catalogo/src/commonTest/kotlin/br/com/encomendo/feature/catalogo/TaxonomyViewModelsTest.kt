package br.com.encomendo.feature.catalogo

import br.com.encomendo.core.domain.catalog.AttributeDraft
import br.com.encomendo.core.domain.catalog.AttributeValueDraft
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

/** Categorias and Campos. */
@OptIn(ExperimentalCoroutinesApi::class) // setMain / resetMain
class TaxonomyViewModelsTest {
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

    @Test
    fun fieldsListWithTheirValues() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            shop.catalog.createAttribute(serveEm())

            val created = shop.attributes()
            val state = shop.settle { created.uiState.value }

            assertEquals(listOf("Serve em"), state.attributes.map { it.name })
            assertEquals(
                listOf("CB 500X", "XRE 300"),
                state.attributes
                    .single()
                    .values
                    .map { it.label },
            )
        }

    @Test
    fun aNewFieldNeedsAUniqueName() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            shop.catalog.createAttribute(serveEm())
            val attributes = shop.attributes()
            shop.settle { }

            attributes.onAddField()
            assertEquals(NameProblem.Missing, attributes.uiState.value.newProblem, "Informe o nome do campo.")
            attributes.onNewNameChange("serve em")
            attributes.onAddField()
            assertEquals(NameProblem.Taken, shop.settle { attributes.uiState.value }.newProblem)

            attributes.onNewNameChange("Cor")
            attributes.onAddField()
            val state = shop.settle { attributes.uiState.value }

            assertEquals(listOf("Serve em", "Cor"), state.attributes.map { it.name })
            assertEquals("", state.newName)
        }

    @Test
    fun aValueIsAddedUnlessEmptyOrRepeated() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val field = shop.catalog.createAttribute(serveEm())
            val attributes = shop.attributes()
            shop.settle { }

            attributes.onAddValue(field)
            assertEquals(ValueProblem.Missing, attributes.uiState.value.valueProblems[field.id], "Informe o valor.")
            attributes.onValueTextChange(field, " cb 500x ")
            attributes.onAddValue(field)
            assertEquals(ValueProblem.Duplicate, attributes.uiState.value.valueProblems[field.id])

            attributes.onValueTextChange(field, "Tenere 700")
            assertNull(attributes.uiState.value.valueProblems[field.id], "typing takes the mark away")
            attributes.onAddValue(field)
            val state = shop.settle { attributes.uiState.value }

            assertEquals(
                listOf("CB 500X", "XRE 300", "Tenere 700"),
                state.attributes
                    .single()
                    .values
                    .map { it.label },
            )
            assertNull(state.valueTexts[field.id])
            assertEquals(listOf<CatalogChange>(CatalogChange.Taxonomy), shop.published)
        }

    @Test
    fun anUnusedValueIsRemovedAtOnceButAUsedOneAsksFirstWithItsCount() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val field = shop.catalog.createAttribute(serveEm())
            // The fake counts nothing: the API's counts are seeded as the server would send them.
            val used = field.values.first().copy(usageCount = 4)
            shop.state.attributes = listOf(field.copy(values = listOf(used, field.values[1])))
            val attributes = shop.attributes()
            val read =
                shop.settle {
                    attributes.uiState.value.attributes
                        .single()
                }

            attributes.onRemoveValue(read, read.values[1])
            val unused = shop.settle { attributes.uiState.value }
            assertEquals(
                listOf("CB 500X"),
                unused.attributes
                    .single()
                    .values
                    .map { it.label },
                "no question for XRE 300",
            )
            assertNull(unused.question)

            attributes.onRemoveValue(unused.attributes.single(), used)
            val asked = shop.settle { attributes.uiState.value }
            assertEquals(
                RemovalQuestion(field.id, used.id, "CB 500X", count = 4),
                asked.question,
                "Ele sai de 4 itens e kits.",
            )
            assertEquals(
                1,
                asked.attributes
                    .single()
                    .values.size,
                "nothing removed yet",
            )

            attributes.onCancelQuestion()
            assertNull(attributes.uiState.value.question)
            assertEquals(
                1,
                shop
                    .settle { attributes.uiState.value }
                    .attributes
                    .single()
                    .values.size,
            )

            attributes.onRemoveValue(unused.attributes.single(), used)
            attributes.onConfirmQuestion()
            val removed = shop.settle { attributes.uiState.value }
            assertTrue(
                removed.attributes
                    .single()
                    .values
                    .isEmpty(),
            )
            assertNull(removed.question)
        }

    @Test
    fun aFieldInUseIsDeletedOnlyAfterTheQuestion() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val field = shop.catalog.createAttribute(serveEm())
            shop.state.attributes = listOf(field.copy(values = field.values.map { it.copy(usageCount = 2) }))
            val attributes = shop.attributes()
            val read =
                shop.settle {
                    attributes.uiState.value.attributes
                        .single()
                }

            attributes.onDeleteField(read)
            val asked = shop.settle { attributes.uiState.value }
            assertEquals(RemovalQuestion(field.id, null, "Serve em", count = 4), asked.question)
            assertEquals(1, shop.state.attributes.size)

            attributes.onConfirmQuestion()
            shop.settle { }

            assertTrue(shop.state.attributes.isEmpty())
            assertTrue(
                attributes.uiState.value.attributes
                    .isEmpty(),
            )
        }

    @Test
    fun aFieldWithNoValuesInUseIsDeletedAtOnce() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val field = shop.catalog.createAttribute(serveEm())
            val attributes = shop.attributes()
            shop.settle { }

            attributes.onDeleteField(field)
            val state = shop.settle { attributes.uiState.value }

            assertNull(state.question)
            assertTrue(state.attributes.isEmpty())
        }

    @Test
    fun aFieldIsRenamed() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val field = shop.catalog.createAttribute(serveEm())
            val attributes = shop.attributes()
            shop.settle { }

            attributes.onStartRename(field)
            attributes.onRenameChange("")
            attributes.onSaveRename()
            assertEquals(
                NameProblem.Missing,
                attributes.uiState.value.rename
                    ?.problem,
            )
            attributes.onRenameChange("Compatível com")
            attributes.onSaveRename()
            val state = shop.settle { attributes.uiState.value }

            assertEquals(listOf("Compatível com"), state.attributes.map { it.name })
            assertNull(state.rename)
            assertEquals(
                2,
                state.attributes
                    .single()
                    .values.size,
                "the values stay",
            )
        }

    @Test
    fun aViewerSeesFieldsAndChangesNone() =
        runTest(main) {
            val shop = Shop(this, Who.Viewer)
            val field = shop.catalog.createAttribute(serveEm())
            val attributes = shop.attributes()
            shop.settle { }

            attributes.onNewNameChange("Cor")
            attributes.onAddField()
            attributes.onDeleteField(field)
            attributes.onStartRename(field)
            shop.settle { }

            assertFalse(attributes.uiState.value.canWrite)
            assertNull(attributes.uiState.value.rename)
            assertEquals(listOf("Serve em"), shop.state.attributes.map { it.name })
        }

    private fun serveEm() =
        AttributeDraft(
            "Serve em",
            listOf(AttributeValueDraft(null, "CB 500X"), AttributeValueDraft(null, "XRE 300")),
        )
}
