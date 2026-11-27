package br.com.encomendo.feature.catalogo

import app.cash.turbine.test
import br.com.encomendo.core.domain.catalog.KitItemDraft
import br.com.encomendo.core.domain.catalog.OwnerField
import br.com.encomendo.core.domain.catalog.SwapDiscount
import br.com.encomendo.core.domain.catalog.isShown
import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.domain.failure.InputIssue
import br.com.encomendo.core.domain.failure.Reference
import br.com.encomendo.core.domain.niche.NicheWords
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
import kotlin.test.assertIs
import kotlin.test.assertNull
import kotlin.test.assertTrue

/** Types a name and a price into the kit editor. */
private fun fillKit(
    form: KitFormViewModel,
    name: String = "Kit Scrambler",
    price: String = "250,00",
) = form.onEdit(null) { it.copy(name = name, price = price) }

/** Adds the item to the kit through the picker, as the user does. */
private fun KitFormViewModel.add(itemId: String) {
    onPickItemToAdd(itemId)
    onAddItem()
}

/** The kit editor: creating, swaps, days, the API's totals, refusals, roles, leaving, photos and the offline copy. */
@OptIn(ExperimentalCoroutinesApi::class) // setMain / resetMain
@Suppress("LargeClass") // One class for the editor, as the item form's tests are split by concern only past this size.
class KitFormViewModelTest {
    private val main = StandardTestDispatcher()

    @BeforeTest
    fun useTestMain() = Dispatchers.setMain(main)

    @AfterTest
    fun restoreMain() = Dispatchers.resetMain()

    @Test
    fun aNewKitWithTwoItemsAndASwappableOneIsSavedWithItsAlternatives() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val bench = shop.seedItem("Banco Scrambler")
            val saddle = shop.seedItem("Selim")
            val otherSaddle = shop.seedItem("Selim de couro")
            val recording = RecordingKits(shop.catalog)
            val form = shop.kitForm(kits = recording)
            shop.settle { }
            fillKit(form)
            form.add(bench.id)
            form.add(saddle.id)
            form.onEdit(null) { it.withRow(bench.id) { row -> row.copy(quantity = "2") } }
            form.onEdit(null) { it.withSwappable(saddle.id, true).withAlternative(saddle.id, otherSaddle.id, true) }

            form.onSave()
            shop.settle { }

            val draft = recording.drafts.single()
            assertEquals("Kit Scrambler", draft.name)
            assertEquals(25_000, draft.priceCents)
            assertEquals(
                listOf(
                    KitItemDraft(bench.id, 2, swappable = false, alternativeItemIds = emptyList()),
                    KitItemDraft(saddle.id, 1, swappable = true, alternativeItemIds = listOf(otherSaddle.id)),
                ),
                draft.items,
            )
        }

    @Test
    fun theEditorStaysOpenOnTheSavedKitWithTheApisTotals() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val bench = shop.seedItem("Banco Scrambler")
            val form = shop.kitForm()
            shop.settle { }
            assertTrue(form.uiState.value.isNew, "Os totais e o prazo aparecem depois de salvar.")
            fillKit(form)
            form.add(bench.id)

            form.effects.test {
                form.onSave()
                shop.settle { }
                expectNoEvents()
            }
            val saved = form.uiState.value

            assertFalse(saved.isNew)
            assertTrue(saved.saved, "Kit salvo.")
            assertEquals(shop.storedKit.id, saved.kit?.id)
            assertFalse(saved.dirty)
            assertEquals(shop.storedKit.id, saved.photos.ownerId, "a new kit can take photos once saved")
            assertTrue(shop.published.contains(CatalogChange.KitSaved))
        }

    @Test
    fun aSecondSaveReplacesTheKitAgainstTheVersionJustSaved() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val bench = shop.seedItem("Banco Scrambler")
            val form = shop.kitForm()
            shop.settle { }
            fillKit(form)
            form.add(bench.id)
            form.onSave()
            shop.settle { }

            form.onEdit(KitField.Name) { it.copy(name = "Kit Scrambler 2") }
            assertTrue(form.uiState.value.dirty)
            form.onSave()
            shop.settle { }

            assertEquals("Kit Scrambler 2", shop.storedKit.name)
            assertEquals(2, shop.storedKit.version)
            assertNull(form.uiState.value.message)
        }

    @Test
    fun bothSwapModesAreSavedAsChosen() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val bench = shop.seedItem("Banco Scrambler")
            val recording = RecordingKits(shop.catalog)
            val form = shop.kitForm(kits = recording)
            shop.settle { }
            assertEquals(SwapDiscount.Keep, form.uiState.value.fields.swapDiscount, "a new kit keeps the discount")
            fillKit(form)
            form.add(bench.id)

            form.onSave()
            shop.settle { }
            form.onEdit(null) { it.copy(swapDiscount = SwapDiscount.Lose) }
            form.onSave()
            shop.settle { }

            assertEquals(listOf(SwapDiscount.Keep, SwapDiscount.Lose), recording.drafts.map { it.swapDiscount })
            assertEquals(SwapDiscount.Lose, shop.storedKit.swapDiscount)
        }

    @Test
    fun blankDaysAreSentAsNullAndANumberAsTheKitsOwn() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val bench = shop.seedItem("Banco Scrambler")
            val recording = RecordingKits(shop.catalog)
            val form = shop.kitForm(kits = recording)
            shop.settle { }
            fillKit(form)
            form.add(bench.id)
            form.onSave()
            shop.settle { }

            form.onEdit(KitField.AssemblyDays) { it.copy(assemblyDays = "5", safetyMarginDays = "0") }
            form.onSave()
            shop.settle { }
            form.onEdit(KitField.AssemblyDays) { it.copy(assemblyDays = "", safetyMarginDays = " ") }
            form.onSave()
            shop.settle { }

            assertEquals(listOf<Int?>(null, 5, null), recording.drafts.map { it.assemblyDays })
            assertEquals(listOf<Int?>(null, 0, null), recording.drafts.map { it.safetyMarginDays })
            assertEquals("", form.uiState.value.fields.assemblyDays, "a kit with no value of its own reads blank")
        }

    @Test
    fun daysOutsideZeroToSixtyAreMarkedAndNothingIsSent() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val bench = shop.seedItem("Banco Scrambler")
            val recording = RecordingKits(shop.catalog)
            val form = shop.kitForm(kits = recording)
            shop.settle { }
            fillKit(form)
            form.add(bench.id)
            form.onEdit(null) { it.copy(assemblyDays = "61", safetyMarginDays = "x") }

            form.onSave()
            val state = shop.settle { form.uiState.value }

            assertEquals(
                FieldIssue.Known(FieldProblem.KitDaysRange),
                state.issue(KitField.AssemblyDays),
                "Use de 0 a 60 dias.",
            )
            assertEquals(FieldIssue.Known(FieldProblem.KitDaysRange), state.issue(KitField.SafetyMarginDays))
            assertEquals(CatalogMessage.CheckFields, state.message)
            assertTrue(recording.drafts.isEmpty())
        }

    @Test
    fun theStoreDefaultsAreReadForTheHintsAndTheNicheWordComesFromTheWordSet() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val form = shop.kitForm(niche = NicheWords("Instalação"))

            val state = shop.settle { form.uiState.value }

            assertEquals(3, state.defaultAssemblyDays, "Padrão da loja: 3 dias")
            assertEquals(2, state.defaultSafetyMarginDays)
            assertEquals("Instalação", state.niche.montagem)
            assertEquals("Montagem", NicheWords.Default.montagem)
        }

    @Test
    fun theEmptyFieldsAreMarkedWithTheDocsWording() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val bench = shop.seedItem("Banco Scrambler")
            val form = shop.kitForm()
            shop.settle { }

            form.onSave()
            val empty = shop.settle { form.uiState.value }
            assertEquals(
                FieldIssue.Known(FieldProblem.KitNameMissing),
                empty.issue(KitField.Name),
                "Informe o nome do kit.",
            )
            assertEquals(
                FieldIssue.Known(FieldProblem.KitPriceMissing),
                empty.issue(KitField.Price),
                "Informe o preço do kit.",
            )
            assertEquals(
                FieldIssue.Known(FieldProblem.KitItemsMissing),
                empty.issue(KitField.Items),
                "Adicione ao menos um item.",
            )

            fillKit(form)
            form.add(bench.id)
            form.onEdit(null) { it.withRow(bench.id) { row -> row.copy(quantity = "100", swappable = true) } }
            form.onSave()
            val rows = shop.settle { form.uiState.value }
            assertEquals(FieldIssue.Known(FieldProblem.KitQuantityRange), rows.issue(KitField.Quantity(bench.id)))
            assertEquals(
                FieldIssue.Known(FieldProblem.KitAlternativesMissing),
                rows.issue(KitField.Alternatives(bench.id)),
                "Escolha ao menos um item para a troca.",
            )
            assertTrue(shop.state.kits.isEmpty())
        }

    @Test
    fun aRowThatIsNotSwappableSendsNoAlternatives() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val bench = shop.seedItem("Banco Scrambler")
            val other = shop.seedItem("Selim")
            val recording = RecordingKits(shop.catalog)
            val form = shop.kitForm(kits = recording)
            shop.settle { }
            fillKit(form)
            form.add(bench.id)
            form.onEdit(null) { it.withSwappable(bench.id, true).withAlternative(bench.id, other.id, true) }
            form.onEdit(null) { it.withSwappable(bench.id, false) }

            form.onSave()
            shop.settle { }

            assertEquals(
                emptyList(),
                recording.drafts
                    .single()
                    .items
                    .single()
                    .alternativeItemIds,
            )
        }

    @Test
    fun theItemPickerOffersOnlyItemsThatAreNotArchivedAndNotInTheKit() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val bench = shop.seedItem("Banco Scrambler")
            val saddle = shop.seedItem("Selim")
            val old = shop.seedItem("Item velho")
            shop.catalog.archiveItem(old.id, old.version)
            val form = shop.kitForm()
            shop.settle { }
            assertEquals(
                listOf("Banco Scrambler", "Selim"),
                form.uiState.value.addable
                    .map { it.name },
            )

            form.add(bench.id)
            assertEquals(
                listOf(saddle.id),
                form.uiState.value.addable
                    .map { it.id },
            )
            assertNull(form.uiState.value.itemToAdd)

            form.onPickItemToAdd(old.id)
            form.onAddItem()
            assertEquals(
                listOf(bench.id),
                form.uiState.value.fields.items
                    .map { it.itemId },
                "an archived item cannot be added",
            )
            form.onEdit(KitField.Items) { it.withoutRow(bench.id) }
            assertTrue(
                form.uiState.value.fields.items
                    .isEmpty(),
                "Remover do kit",
            )
        }

    @Test
    fun sameCategorySuggestionsComeFirstAndTheRestFollow() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            shop.state.categories = listOf(category("cat-seats", "Bancos"), category("cat-other", "Peças"))
            val bench = shop.seedItem("Banco Scrambler") { copy(categoryId = "cat-seats") }
            val bench2 = shop.seedItem("Banco rally") { copy(categoryId = "cat-seats") }
            val axle = shop.seedItem("Eixo") { copy(categoryId = "cat-other") }
            val old = shop.seedItem("Banco velho") { copy(categoryId = "cat-seats") }
            shop.catalog.archiveItem(old.id, old.version)
            val form = shop.kitForm()
            shop.settle { }

            val state = form.uiState.value
            val groups = alternativesFor(bench.id, emptyList(), state.catalogItems, state.categories)

            assertEquals(
                listOf(AlternativeHeading.Suggestions("Bancos"), AlternativeHeading.Others),
                groups.map { it.heading },
                "Sugestões da categoria Bancos, then Outros itens",
            )
            assertEquals(listOf(bench2.id), groups[0].items.map { it.id })
            assertEquals(listOf(axle.id), groups[1].items.map { it.id }, "the archived item is not offered")
        }

    @Test
    fun anArchivedAlternativeStaysListedWhileTheRowNamesIt() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            shop.state.categories = listOf(category("cat-seats", "Bancos"))
            val bench = shop.seedItem("Banco Scrambler") { copy(categoryId = "cat-seats") }
            val old = shop.seedItem("Banco velho") { copy(categoryId = "cat-seats") }
            shop.catalog.archiveItem(old.id, old.version)
            val form = shop.kitForm()
            shop.settle { }
            val state = form.uiState.value

            val groups = alternativesFor(bench.id, listOf(old.id), state.catalogItems, state.categories)

            assertEquals(
                listOf(old.id),
                groups.single().items.map { it.id },
                "listed as (arquivado), so it can be unticked",
            )
            assertTrue(
                groups
                    .single()
                    .items
                    .single()
                    .archived,
            )
        }

    @Test
    fun withoutACategoryTheAlternativesAreOneFlatList() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val bench = shop.seedItem("Banco Scrambler")
            val saddle = shop.seedItem("Selim")
            val form = shop.kitForm()
            shop.settle { }
            val state = form.uiState.value

            val groups = alternativesFor(bench.id, emptyList(), state.catalogItems, state.categories)

            assertEquals(1, groups.size)
            assertNull(groups.single().heading)
            assertEquals(listOf(saddle.id), groups.single().items.map { it.id })
        }

    @Test
    fun theTotalsAreTheApisAndStayThatWayWhileTheFormHasEdits() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val bench = shop.seedItem("Banco Scrambler")
            val kit = shop.seedKit("Kit Scrambler", listOf(plainRow(bench.id)))
            // Numbers the screen could never work out from the form: they must be shown exactly as sent.
            val sent =
                kit.copy(
                    singleItemsTotal = kit.singleItemsTotal.copy(cents = 31_337),
                    discount = kit.discount.copy(cents = 6_337),
                    days = kit.days.copy(slowestItem = 9, assembly = 4, safetyMargin = 1, total = 14),
                )
            shop.state.kits = listOf(sent)
            val form = shop.kitForm(kit.id)
            shop.settle { }
            assertFalse(form.uiState.value.dirty)

            form.onEdit(KitField.Price) { it.copy(price = "1,00", assemblyDays = "30") }
            val edited = form.uiState.value

            assertTrue(edited.dirty, "Os totais e o prazo mostram o último salvamento.")
            assertEquals(sent.singleItemsTotal, edited.kit?.singleItemsTotal)
            assertEquals(sent.discount, edited.kit?.discount)
            assertEquals(sent.days, edited.kit?.days)
        }

    @Test
    fun aKitPricedAboveItsItemsShowsTheWarningFromTheApisDiscount() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val bench = shop.seedItem("Banco Scrambler") // R$ 100,00
            val above = shop.seedKit("Kit caro", listOf(plainRow(bench.id)), configure = { copy(priceCents = 30_000) })
            val below = shop.seedKit("Kit barato", listOf(plainRow(bench.id)), configure = { copy(priceCents = 5_000) })

            val warned =
                shop
                    .kitForm(above.id)
                    .also { shop.settle { } }
                    .uiState.value
            val calm =
                shop
                    .kitForm(below.id)
                    .also { shop.settle { } }
                    .uiState.value

            assertTrue(warned.priceAboveItems, "O preço do kit está acima da soma avulsa.")
            assertFalse(calm.priceAboveItems)
            assertTrue(
                shop.catalog
                    .kit(above.id)
                    .value.discount.cents < 0,
            )
        }

    @Test
    fun theOwnersKitCarriesAMarginAndAnAdminsDoesNot() =
        runTest(main) {
            val owner = Shop(this, Who.Owner)
            val bench = owner.seedItem("Banco Scrambler")
            val ownerKit = owner.seedKit("Kit Scrambler", listOf(plainRow(bench.id)))
            val admin = Shop(this, Who.Admin)
            val adminBench = admin.seedItem("Banco Scrambler")
            val adminKit = admin.seedKit("Kit Scrambler", listOf(plainRow(adminBench.id)))

            val ownerState =
                owner
                    .kitForm(ownerKit.id)
                    .also { owner.settle { } }
                    .uiState.value
            val adminState =
                admin
                    .kitForm(adminKit.id)
                    .also { admin.settle { } }
                    .uiState.value

            assertTrue(ownerState.kit?.margin?.isShown == true, "Margem: — when the Dono's kit has no cost")
            assertEquals(OwnerField.Hidden, adminState.kit?.margin)
        }

    @Test
    fun anArchivedItemTheApiNamesIsReportedOncePerItem() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val bench = shop.seedItem("Banco Scrambler")
            val recording = RecordingKits(shop.catalog)
            val form = shop.kitForm(kits = recording)
            shop.settle { }
            fillKit(form)
            form.add(bench.id)
            recording.refuse =
                Failure.ItemArchived(
                    listOf(Reference("a", "Banco"), Reference("a", "Banco"), Reference("b", "Selim")),
                )

            form.onSave()
            val state = shop.settle { form.uiState.value }

            assertEquals(CatalogMessage.KitItemsArchived(listOf("Banco", "Selim")), state.message)
            assertFalse(state.saved)
            assertEquals("Kit Scrambler", state.fields.name, "what was typed stays")
        }

    @Test
    fun anArchivedItemTheApiDoesNotNameIsReportedInGeneral() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val bench = shop.seedItem("Banco Scrambler")
            val recording = RecordingKits(shop.catalog)
            val form = shop.kitForm(kits = recording)
            shop.settle { }
            fillKit(form)
            form.add(bench.id)
            recording.refuse = Failure.ItemArchived(emptyList())

            form.onSave()

            assertEquals(CatalogMessage.KitItemsArchived(emptyList()), shop.settle { form.uiState.value.message })
        }

    @Test
    fun restoringAKitWithAnArchivedItemSaysTheSameAndStaysOpen() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val bench = shop.seedItem("Banco Scrambler")
            val kit = shop.seedKit("Kit Scrambler", listOf(plainRow(bench.id)))
            shop.catalog.archiveKit(kit.id, kit.version)
            shop.state.items = shop.state.items.map { it.copy(archived = true) }
            val form = shop.kitForm(kit.id)
            shop.settle { }
            assertTrue(form.uiState.value.archived)

            form.effects.test {
                form.onRestore()
                shop.settle { }
                expectNoEvents()
            }

            assertEquals(CatalogMessage.KitItemsArchived(listOf("Banco Scrambler")), form.uiState.value.message)
            assertTrue(shop.storedKit.archived)
        }

    @Test
    fun theServersWordsForAFieldShowNextToIt() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val bench = shop.seedItem("Banco Scrambler")
            val recording = RecordingKits(shop.catalog)
            val form = shop.kitForm(kits = recording)
            shop.settle { }
            fillKit(form)
            form.add(bench.id)
            recording.refuse =
                Failure.InvalidInput(
                    listOf(
                        InputIssue("priceCents", "Use um valor de até R$ 1.000.000.000,00."),
                        InputIssue("items[0].quantity", "Use uma quantidade de 1 a 99."),
                        InputIssue("somethingElse", "Algo mais."),
                    ),
                )

            form.onSave()
            val state = shop.settle { form.uiState.value }

            assertEquals(FieldIssue.Server("Use um valor de até R$ 1.000.000.000,00."), state.issue(KitField.Price))
            assertEquals(FieldIssue.Server("Use uma quantidade de 1 a 99."), state.issue(KitField.Quantity(bench.id)))
            assertEquals(CatalogMessage.General(FailureMessage.Refused("Algo mais.")), state.message)
        }

    @Test
    fun someoneElseSavingFirstIsAConflictAndRecarregarReadsTheKitAgain() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val bench = shop.seedItem("Banco Scrambler")
            val kit = shop.seedKit("Kit Scrambler", listOf(plainRow(bench.id)))
            val form = shop.kitForm(kit.id)
            shop.settle { }
            form.onEdit(KitField.Name) { it.copy(name = "Meu nome") }
            shop.catalog.setKitPublished(kit.id, true, kit.version)

            form.onSave()
            val conflict = shop.settle { form.uiState.value }
            assertEquals(CatalogMessage.Conflict, conflict.message, "Outra pessoa alterou este kit.")
            assertEquals("Meu nome", conflict.fields.name)

            form.onReload()
            val reloaded = shop.settle { form.uiState.value }
            assertEquals("Kit Scrambler", reloaded.fields.name, "Recarregar drops the edits that lost")
            assertTrue(reloaded.fields.published)
            assertNull(reloaded.message)
            assertEquals(2, reloaded.kit?.version)
        }

    @Test
    fun aViewerOpensTheKitReadOnlyAndCannotChangeIt() =
        runTest(main) {
            val shop = Shop(this, Who.Viewer)
            val bench = shop.seedItem("Banco Scrambler")
            val kit = shop.seedKit("Kit Scrambler", listOf(plainRow(bench.id)))
            val form = shop.kitForm(kit.id)
            shop.settle { }

            form.onEdit(KitField.Name) { it.copy(name = "Outro") }
            form.onPickItemToAdd(bench.id)
            form.onSave()
            form.onArchive()
            shop.settle { }
            val state = form.uiState.value

            assertFalse(state.canWrite, "Kit, with Fechar")
            assertFalse(state.editable)
            assertEquals("Kit Scrambler", state.fields.name)
            assertNull(state.itemToAdd)
            assertEquals("Kit Scrambler", shop.storedKit.name)
            assertFalse(shop.storedKit.archived)
            assertEquals(1, shop.storedKit.version)
        }

    @Test
    fun cancelWithNothingTypedClosesAtOnceAndWithEditsAsksFirst() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val bench = shop.seedItem("Banco Scrambler")
            val kit = shop.seedKit("Kit Scrambler", listOf(plainRow(bench.id)))
            val form = shop.kitForm(kit.id)
            shop.settle { }

            form.effects.test {
                form.onCancel()
                shop.settle { }
                assertEquals(KitFormEffect.Close, awaitItem())

                form.onEdit(KitField.Name) { it.copy(name = "Outro") }
                form.onCancel()
                assertEquals(DiscardAction.Close, form.uiState.value.discard, "Descartar as alterações?")
                form.onKeepEditing()
                assertNull(form.uiState.value.discard)
                form.onCancel()
                form.onDiscard()
                shop.settle { }
                assertEquals(KitFormEffect.Close, awaitItem())
            }
            assertEquals("Kit Scrambler", shop.storedKit.name)
        }

    @Test
    fun archivingWithEditsAsksFirstAndDiscardingArchivesTheSavedKit() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val bench = shop.seedItem("Banco Scrambler")
            val kit = shop.seedKit("Kit Scrambler", listOf(plainRow(bench.id)))
            val form = shop.kitForm(kit.id)
            shop.settle { }
            form.onEdit(KitField.Name) { it.copy(name = "Outro") }

            form.effects.test {
                form.onArchive()
                shop.settle { }
                assertEquals(DiscardAction.Archive, form.uiState.value.discard)
                assertFalse(shop.storedKit.archived, "nothing happens before the answer")
                expectNoEvents()

                form.onDiscard()
                shop.settle { }
                assertEquals(KitFormEffect.Close, awaitItem())
            }

            assertTrue(shop.storedKit.archived)
            assertEquals("Kit Scrambler", shop.storedKit.name, "the edits were not saved")
            assertEquals(CatalogChange.KitArchived("Kit Scrambler"), shop.published.last())
        }

    @Test
    fun archivingWithNothingTypedArchivesAtOnceAndRestoreBringsItBack() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val bench = shop.seedItem("Banco Scrambler")
            val kit = shop.seedKit("Kit Scrambler", listOf(plainRow(bench.id)))
            val form = shop.kitForm(kit.id)
            shop.settle { }

            form.effects.test {
                form.onArchive()
                shop.settle { }
                assertEquals(KitFormEffect.Close, awaitItem())
            }
            val again = shop.kitForm(kit.id)
            shop.settle { }
            again.effects.test {
                again.onRestore()
                shop.settle { }
                assertEquals(KitFormEffect.Close, awaitItem())
            }

            assertFalse(shop.storedKit.archived)
            assertEquals(CatalogChange.KitRestored("Kit Scrambler"), shop.published.last())
        }

    @Test
    fun aNewKitHasNoPhotosUntilItIsSaved() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val form = shop.kitForm()
            shop.settle { }
            shop.picker.gallery += jpeg()

            form.onAddPhoto(PhotoSource.Gallery)
            val state = shop.settle { form.uiState.value }

            assertNull(state.photos.ownerId, "Salve o kit para adicionar fotos.")
            assertEquals(0, shop.picker.galleryOpened)
        }

    @Test
    fun kitPhotosGoThroughThePhotoControllerAtOnce() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val bench = shop.seedItem("Banco Scrambler")
            val kit = shop.seedKit("Kit Scrambler", listOf(plainRow(bench.id)))
            val form = shop.kitForm(kit.id)
            shop.settle { }
            shop.picker.gallery += jpeg()
            shop.picker.camera += jpeg()

            form.onAddPhoto(PhotoSource.Gallery)
            shop.settle { }
            form.onAddPhoto(PhotoSource.Camera)
            val two = shop.settle { form.uiState.value }
            assertEquals(2, two.photos.photos.size)
            assertEquals(2, shop.storedKit.photos.size, "uploaded to the kit, not saved with Salvar")
            assertTrue(shop.storedItemsPhotos().isEmpty(), "and not to an item")

            val second = two.photos.photos[1]
            form.onMakeCover(second)
            val ordered = shop.settle { form.uiState.value }
            assertEquals(
                second.id,
                ordered.photos.photos
                    .first()
                    .id,
                "Usar como capa",
            )

            form.onRemovePhoto(ordered.photos.photos.last())
            shop.settle { }
            assertEquals(1, shop.storedKit.photos.size)
            assertEquals(1, form.uiState.value.photos.photos.size)
        }

    @Test
    fun aKitCannotHoldMoreThanEightPhotosAndSaysSoInKitWording() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val bench = shop.seedItem("Banco Scrambler")
            val kit = shop.seedKit("Kit Scrambler", listOf(plainRow(bench.id)))
            val form = shop.kitForm(kit.id)
            shop.settle { }
            repeat(MAX_PHOTOS) {
                shop.picker.gallery += jpeg()
                form.onAddPhoto(PhotoSource.Gallery)
                shop.settle { }
            }

            form.onAddPhoto(PhotoSource.Gallery)
            val state = shop.settle { form.uiState.value }

            assertEquals(CatalogMessage.PhotoLimit, state.photos.message, "Cada kit pode ter até 8 fotos.")
            assertEquals(MAX_PHOTOS, state.photos.photos.size)
        }

    @Test
    fun withNoConnectionAKitFromTheDevicesCopyOpensReadOnlyWithItsDate() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val bench = shop.seedItem("Banco Scrambler")
            val kit = shop.seedKit("Kit Scrambler", listOf(plainRow(bench.id)))
            shop.state.offlineSince = SAVED_AT
            shop.settings.failure = Failure.Offline()
            val form = shop.kitForm(kit.id)

            val state = shop.settle { form.uiState.value }
            form.onEdit(KitField.Name) { it.copy(name = "Outro") }
            form.onSave()
            shop.settle { }

            assertEquals(SAVED_AT, state.offlineSince)
            assertFalse(state.canWrite, "the editor could not save anyway")
            assertEquals("Kit Scrambler", state.kit?.name)
            assertEquals("Kit Scrambler", form.uiState.value.fields.name)
            assertEquals("Kit Scrambler", shop.storedKit.name)
        }

    @Test
    fun aNewKitWithNoConnectionSaysSoAndOffersToTryAgain() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            shop.settings.failure = Failure.Offline()
            val form = shop.kitForm()

            val failed = shop.settle { form.uiState.value }
            assertEquals(CatalogMessage.General(FailureMessage.Offline), failed.loadMessage)

            shop.settings.failure = null
            form.onReload()
            val loaded = shop.settle { form.uiState.value }
            assertNull(loaded.loadMessage)
            assertTrue(loaded.editable)
        }

    @Test
    fun aKitWhoseItemWasArchivedLaterShowsTheRowMarkedAndRefusesToSave() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val bench = shop.seedItem("Banco Scrambler")
            val kit = shop.seedKit("Kit Scrambler", listOf(plainRow(bench.id)))
            // The server flags the kit's row when its item is archived after the kit was saved.
            shop.state.items = shop.state.items.map { it.copy(archived = true) }
            shop.state.kits = shop.state.kits.map { k -> k.copy(items = k.items.map { it.copy(archived = true) }) }
            val form = shop.kitForm(kit.id)
            shop.settle { }
            assertTrue(
                form.uiState.value.fields.items
                    .single()
                    .archived,
                "Arquivado",
            )

            form.onSave()
            shop.settle { }
            assertIs<CatalogMessage.KitItemsArchived>(form.uiState.value.message)

            form.onEdit(KitField.Items) { it.withoutRow(bench.id) }
            assertTrue(
                form.uiState.value.fields.items
                    .isEmpty(),
            )
        }
}

private const val MAX_PHOTOS = 8

private fun category(
    id: String,
    name: String,
) = br.com.encomendo.core.domain.catalog
    .Category(id, name, position = 0)

private fun Shop.storedItemsPhotos() = state.items.flatMap { it.photos }
