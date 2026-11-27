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

/** Campos: each field is a draft until "Salvar campo". */
@OptIn(ExperimentalCoroutinesApi::class) // setMain / resetMain
class AttributesViewModelTest {
    private val main = StandardTestDispatcher()

    @BeforeTest
    fun useTestMain() = Dispatchers.setMain(main)

    @AfterTest
    fun restoreMain() = Dispatchers.resetMain()

    @Test
    fun fieldsListWithTheirValuesAsDrafts() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            shop.catalog.createAttribute(serveEm())
            val attributes = shop.attributes()

            val state = shop.settle { attributes.uiState.value }

            assertEquals(listOf("Serve em"), state.editors.map { it.name })
            assertEquals(
                listOf("CB 500X", "XRE 300"),
                state.editor
                    .values
                    .map { it.label },
            )
            assertFalse(state.loading)
            assertTrue(state.canWrite)
        }

    @Test
    fun nothingIsSavedUntilSalvarCampo() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            shop.catalog.createAttribute(serveEm())
            val attributes = shop.attributes()
            val editor =
                shop.settle {
                    attributes.editor
                }

            attributes.onNameChange(editor, "Compatível com")
            attributes.onAddValue(editor)
            val added =
                attributes.editor
            attributes.onValueChange(added, added.values.last(), "Tenere 700")
            attributes.onRemoveValue(added, added.values.first())
            shop.settle { }

            assertEquals(
                "Serve em",
                shop.storedAttribute
                    .name,
                "still as the server has it",
            )
            assertEquals(
                2,
                shop.storedAttribute
                    .values.size,
            )
            assertTrue(shop.published.isEmpty())

            attributes.onSaveField(
                attributes.editor,
            )
            val saved = shop.settle { attributes.uiState.value }

            assertEquals(
                "Compatível com",
                shop.storedAttribute
                    .name,
            )
            assertEquals(
                listOf("XRE 300", "Tenere 700"),
                shop.storedAttribute
                    .values
                    .map { it.label },
            )
            assertEquals(
                listOf("XRE 300", "Tenere 700"),
                saved.editor
                    .values
                    .map { it.label },
            )
            assertEquals(listOf<CatalogChange>(CatalogChange.Taxonomy), shop.published)
            assertNull(saved.savingKey)
        }

    @Test
    fun aNewFieldIsADraftSavedWithItsValues() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val attributes = shop.attributes()
            shop.settle { }

            attributes.onNewField()
            val blank =
                attributes.editor
            assertNull(blank.id)
            attributes.onNameChange(blank, "Cor")
            attributes.onAddValue(
                attributes.editor,
            )
            val withValue =
                attributes.editor
            attributes.onValueChange(withValue, withValue.values.single(), "Preta")
            attributes.onSaveField(
                attributes.editor,
            )
            val state = shop.settle { attributes.uiState.value }

            assertEquals(listOf("Cor"), shop.state.attributes.map { it.name })
            assertEquals(
                listOf("Preta"),
                shop.storedAttribute
                    .values
                    .map { it.label },
            )
            assertEquals(
                shop.storedAttribute
                    .id,
                state.editor.id,
                "the draft now knows its field",
            )
        }

    @Test
    fun emptyAndRepeatedFieldsAreMarkedAndNothingIsSent() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val attributes = shop.attributes()
            shop.settle { }
            attributes.onNewField()
            attributes.onAddValue(
                attributes.editor,
            )
            attributes.onAddValue(
                attributes.editor,
            )

            attributes.onSaveField(
                attributes.editor,
            )
            val empty =
                attributes.editor
            assertEquals(NameProblem.Missing, empty.nameProblem, "Informe o nome do campo.")
            assertEquals(listOf(ValueProblem.Missing, ValueProblem.Missing), empty.values.map { it.problem })

            attributes.onNameChange(empty, "Cor")
            attributes.onValueChange(empty, empty.values[0], "Preta")
            attributes.onValueChange(empty, empty.values[1], " preta ")
            attributes.onSaveField(
                attributes.editor,
            )
            val repeated =
                shop.settle {
                    attributes.editor
                }

            assertEquals(listOf(null, ValueProblem.Duplicate), repeated.values.map { it.problem })
            assertTrue(shop.state.attributes.isEmpty(), "nothing was sent")
            attributes.onValueChange(repeated, repeated.values[1], "Branca")
            assertNull(
                attributes.editor
                    .values[1]
                    .problem,
                "typing takes the mark away",
            )
        }

    @Test
    fun aFieldNameAlreadyUsedIsMarkedNextToTheName() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            shop.catalog.createAttribute(serveEm())
            val attributes = shop.attributes()
            shop.settle { }
            attributes.onNewField()
            val blank =
                attributes.uiState.value.editors
                    .last()
            attributes.onNameChange(blank, "serve em")

            attributes.onSaveField(
                attributes.uiState.value.editors
                    .last(),
            )
            val state = shop.settle { attributes.uiState.value }

            assertEquals(NameProblem.Taken, state.editors.last().nameProblem, "Já existe um campo com este nome.")
            assertNull(state.message)
            assertEquals(1, shop.state.attributes.size)
        }

    @Test
    fun anUnusedValueLeavesTheDraftAtOnce() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            shop.catalog.createAttribute(serveEm())
            val attributes = shop.attributes()
            val editor = shop.settle { attributes.editor }

            attributes.onRemoveValue(editor, editor.values[1])
            val state = attributes.uiState.value

            assertEquals(listOf("CB 500X"), state.editor.labels, "no question for a value nobody picked")
            assertNull(state.question)
        }

    @Test
    fun aUsedValueAsksFirstWithItsCountAndLeavesOnlyOnConfirm() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val field = shop.catalog.createAttribute(serveEm())
            // The fake counts nothing: the API's counts are seeded as the server would send them.
            val used = field.values.first().copy(usageCount = 4)
            shop.state.attributes = listOf(field.copy(values = listOf(used)))
            val attributes = shop.attributes()
            val editor = shop.settle { attributes.editor }

            attributes.onRemoveValue(editor, editor.values.single())
            val asked = attributes.uiState.value
            val expected = RemovalQuestion.OfValue(editor.key, valueKey = 1, name = "CB 500X", count = 4)
            assertEquals(expected, asked.question, "Remover CB 500X? Ele sai de 4 itens e kits.")
            assertEquals(listOf("CB 500X"), asked.editor.labels, "nothing removed yet")

            attributes.onCancelQuestion()
            assertNull(attributes.uiState.value.question)
            assertEquals(listOf("CB 500X"), attributes.editor.labels)

            attributes.onRemoveValue(editor, editor.values.single())
            attributes.onConfirmQuestion()
            val removed = attributes.uiState.value
            assertTrue(removed.editor.values.isEmpty(), "gone from the draft")
            assertNull(removed.question)
            assertEquals(listOf("CB 500X"), shop.storedAttribute.values.map { it.label }, "still on the server")
        }

    @Test
    fun aFieldWhoseValuesAreInUseAsksWithoutACountEvenWhenOneItemPicksTwoOfThem() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val field = shop.catalog.createAttribute(serveEm())
            val bothValues = field.values.map { it.id }
            shop.seedItem("Banco Scrambler") { copy(attributeValueIds = bothValues) }
            // One item picks both values, so the API counts it once for each value: adding them would say 2.
            shop.state.attributes = listOf(field.copy(values = field.values.map { it.copy(usageCount = 1) }))
            val attributes = shop.attributes()
            val editor =
                shop.settle {
                    attributes.editor
                }

            attributes.onDeleteField(editor)
            val asked = attributes.uiState.value

            assertEquals(RemovalQuestion.OfField(editor.key, "Serve em"), asked.question)
            assertEquals(1, shop.state.attributes.size, "nothing deleted before the answer")

            attributes.onConfirmQuestion()
            val deleted = shop.settle { attributes.uiState.value }

            assertTrue(deleted.editors.isEmpty())
            assertTrue(shop.state.attributes.isEmpty())
            assertEquals(listOf<CatalogChange>(CatalogChange.Taxonomy), shop.published)
        }

    @Test
    fun aFieldWithNoValueInUseIsDeletedAtOnceAndANewDraftJustGoes() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            shop.catalog.createAttribute(serveEm())
            val attributes = shop.attributes()
            val editor =
                shop.settle {
                    attributes.editor
                }
            attributes.onNewField()

            attributes.onDeleteField(
                attributes.uiState.value.editors
                    .last(),
            )
            assertEquals(1, attributes.uiState.value.editors.size, "the unsaved draft needs no call")
            attributes.onDeleteField(editor)
            val state = shop.settle { attributes.uiState.value }

            assertNull(state.question)
            assertTrue(state.editors.isEmpty())
            assertTrue(shop.state.attributes.isEmpty())
        }

    @Test
    fun aViewerSeesFieldsAndChangesNone() =
        runTest(main) {
            val shop = Shop(this, Who.Viewer)
            val field = shop.catalog.createAttribute(serveEm())
            val attributes = shop.attributes()
            val editor =
                shop.settle {
                    attributes.editor
                }

            attributes.onNewField()
            attributes.onNameChange(editor, "Outro")
            attributes.onAddValue(editor)
            attributes.onSaveField(editor)
            attributes.onDeleteField(editor)
            shop.settle { }

            assertFalse(attributes.uiState.value.canWrite)
            assertEquals(
                editor,
                attributes.editor,
            )
            assertEquals(listOf(field.name), shop.state.attributes.map { it.name })
        }

    @Test
    fun fieldsThatCannotBeReadSayWhy() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            shop.state.failure = Failure.Offline()
            val attributes = shop.attributes()

            val state = shop.settle { attributes.uiState.value }

            assertEquals(CatalogMessage.General(FailureMessage.Offline), state.message)
        }

    private fun serveEm() =
        AttributeDraft(
            "Serve em",
            listOf(AttributeValueDraft(null, "CB 500X"), AttributeValueDraft(null, "XRE 300")),
        )
}
