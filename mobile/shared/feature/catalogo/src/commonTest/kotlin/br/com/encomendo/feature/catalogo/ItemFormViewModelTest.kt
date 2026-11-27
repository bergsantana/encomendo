package br.com.encomendo.feature.catalogo

import app.cash.turbine.test
import br.com.encomendo.core.domain.catalog.CostChange
import br.com.encomendo.core.domain.catalog.KitDraft
import br.com.encomendo.core.domain.catalog.KitItemDraft
import br.com.encomendo.core.domain.catalog.SourcingMode
import br.com.encomendo.core.domain.catalog.SupplierLinkDraft
import br.com.encomendo.core.domain.catalog.SwapDiscount
import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.domain.failure.InputIssue
import br.com.encomendo.core.domain.settings.StoreSettings
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

@OptIn(ExperimentalCoroutinesApi::class) // setMain / resetMain
class ItemFormViewModelTest {
    private val main = StandardTestDispatcher()

    @BeforeTest
    fun useTestMain() = Dispatchers.setMain(main)

    @AfterTest
    fun restoreMain() = Dispatchers.resetMain()

    @Test
    fun aNewItemFormForTheOwnerOffersEverySourceSupplierAndCostFields() =
        runTest(main) {
            val shop = Shop(this, Who.Owner)
            val form = shop.form()

            val state = shop.settle { form.uiState.value }

            assertTrue(state.isNew)
            assertTrue(state.editable)
            assertTrue(state.showsCosts, "the owner is sent costs: Custo shows")
            assertEquals(SourcingMode.entries, state.modes)
            assertEquals(listOf("Couros Paulista", "Eixo Forte"), state.suppliers.map { it.name })
        }

    @Test
    fun anAdminsNewItemFormHasNoCostField() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)

            val created = shop.form()
            val state = shop.settle { created.uiState.value }

            assertTrue(state.editable)
            assertFalse(state.showsCosts)
        }

    @Test
    fun originIsFilteredByWhatTheStoreTurnedOn() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            shop.settings.current = StoreSettings(stockEnabled = false, productionEnabled = false, 3, 2)

            val created = shop.form()
            val state = shop.settle { created.uiState.value }

            assertEquals(listOf(SourcingMode.ToOrder), state.modes, "Estoque and Produção própria are off")
        }

    @Test
    fun anItemKeepsItsOwnOriginEvenWhenTheStoreTurnedItOff() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val stocked = shop.seedItem("Banco Scrambler") { copy(sourcingMode = SourcingMode.Stock) }
            shop.settings.current = StoreSettings(stockEnabled = false, productionEnabled = false, 3, 2)

            val created = shop.form(stocked.id)
            val state = shop.settle { created.uiState.value }

            assertEquals(listOf(SourcingMode.ToOrder, SourcingMode.Stock), state.modes)
            assertEquals(SourcingMode.Stock, state.fields.mode)
        }

    @Test
    fun emptyFieldsAreMarkedAndNothingIsSent() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val items = RecordingItems(shop.catalog)
            val form = shop.form(items = items)
            shop.settle { }

            form.onSave()
            val state = shop.settle { form.uiState.value }

            assertTrue(items.drafts.isEmpty())
            assertEquals(CatalogMessage.CheckFields, state.message, "Confira os campos destacados.")
            assertEquals(FieldIssue.Known(FieldProblem.NameMissing), state.issue(ItemField.Name))
            assertEquals(FieldIssue.Known(FieldProblem.PriceMissing), state.issue(ItemField.Price))
            assertEquals(
                FieldIssue.Known(FieldProblem.MainSupplierMissing),
                state.issue(ItemField.Supplier(SupplierSlot.Main)),
            )
            assertEquals(
                FieldIssue.Known(FieldProblem.LeadDaysMissing),
                state.issue(ItemField.LeadDays(SupplierSlot.Main)),
            )
            assertFalse(state.saving)

            form.onEdit(ItemField.Name) { it.copy(name = "Banco") }
            assertNull(form.uiState.value.issue(ItemField.Name), "typing in a field takes its mark away")
            assertNotNullIssue(form, ItemField.Price)
        }

    @Test
    fun anOwnerSavesANewItemWithItsCostAndTheListLearnsOfIt() =
        runTest(main) {
            val shop = Shop(this, Who.Owner)
            val items = RecordingItems(shop.catalog)
            val form = shop.form(items = items)
            shop.settle { }
            fill(form, name = "Banco Scrambler", price = "1.234,50")
            form.onEdit(null) { f ->
                f
                    .withSupplier(SupplierSlot.Main, SUPPLIER_ID)
                    .withSlot(SupplierSlot.Main) { it.copy(leadDays = "15", cost = "412,00", costTouched = true) }
            }

            form.effects.test {
                form.onSave()
                shop.settle { }
                assertEquals(ItemFormEffect.Close, awaitItem())
            }

            val sent = items.drafts.single()
            assertEquals(123_450, sent.priceCents)
            assertEquals(SupplierLinkDraft(SUPPLIER_ID, 15, CostChange.Set(41_200)), sent.mainSupplier)
            assertEquals(listOf("Banco Scrambler"), shop.state.items.map { it.name })
            assertEquals(listOf<CatalogChange>(CatalogChange.ItemSaved), shop.published)
        }

    @Test
    fun anAdminsSaveLeavesEveryCostOutSoTheServerKeepsIt() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val seeded = shop.seedItem("Banco Scrambler") { copy(sourcingMode = SourcingMode.ToOrder) }
            val items = RecordingItems(shop.catalog)
            val form = shop.form(seeded.id, items)
            val loaded = shop.settle { form.uiState.value }
            assertFalse(loaded.showsCosts)
            assertEquals("", loaded.fields.main.cost)

            form.onEdit(ItemField.Name) { it.copy(name = "Banco Scrambler Preto") }
            form.onSave()
            shop.settle { }

            val sent = items.drafts.single()
            assertEquals(CostChange.Keep, sent.mainSupplier?.cost)
            assertEquals(CostChange.Keep, sent.productionCost)
            assertEquals(
                "Banco Scrambler Preto",
                shop.storedItem
                    .name,
            )
        }

    @Test
    fun anOwnersUntouchedCostIsKeptAndAClearedOneIsCleared() =
        runTest(main) {
            val shop = Shop(this, Who.Owner)
            val seeded = shop.seedItem("Banco Scrambler")
            val items = RecordingItems(shop.catalog)
            val form = shop.form(seeded.id, items)
            val loaded = shop.settle { form.uiState.value }
            assertEquals("40,00", loaded.fields.main.cost, "the cost the API sent")

            form.onSave()
            shop.settle { }
            form.onReload()
            shop.settle { }
            form.onEdit(null) { f -> f.withSlot(SupplierSlot.Main) { it.copy(cost = "", costTouched = true) } }
            form.onSave()
            shop.settle { }

            assertEquals(listOf(CostChange.Keep, CostChange.Clear), items.drafts.map { it.mainSupplier?.cost })
        }

    @Test
    fun aDifferentSupplierStartsWithoutTheOldCost() =
        runTest(main) {
            val shop = Shop(this, Who.Owner)
            val seeded = shop.seedItem("Banco Scrambler")
            val form = shop.form(seeded.id)
            shop.settle { }

            form.onEdit(ItemField.Supplier(SupplierSlot.Main)) { it.withSupplier(SupplierSlot.Main, OTHER_SUPPLIER_ID) }

            val main = form.uiState.value.fields.main
            assertEquals(OTHER_SUPPLIER_ID, main.supplierId)
            assertEquals("", main.cost, "a cost belongs to the item and one supplier (ADR 0025)")
            assertFalse(main.costTouched)
        }

    @Test
    fun anInHouseItemNeedsProductionDaysAndSendsNoSupplier() =
        runTest(main) {
            val shop = Shop(this, Who.Owner)
            val items = RecordingItems(shop.catalog)
            val form = shop.form(items = items)
            shop.settle { }
            fill(form, name = "Capa de banco", price = "80")
            form.onModeChange(SourcingMode.InHouse)

            form.onSave()
            shop.settle { }
            assertEquals(
                FieldIssue.Known(FieldProblem.ProductionDaysMissing),
                form.uiState.value.issue(ItemField.ProductionDays),
            )
            form.onEdit(ItemField.ProductionDays) {
                it.copy(productionDays = "4", productionCost = "12,5", productionCostTouched = true)
            }
            form.onSave()
            shop.settle { }

            val sent = items.drafts.single()
            assertEquals(SourcingMode.InHouse, sent.sourcingMode)
            assertEquals(4, sent.productionDays)
            assertEquals(CostChange.Set(1_250), sent.productionCost)
            assertNull(sent.mainSupplier)
        }

    @Test
    fun theBackupSupplierMustBeAnotherOne() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val form = shop.form()
            shop.settle { }
            fill(form, name = "Banco", price = "100")
            form.onEdit(null) { f ->
                f
                    .withSupplier(SupplierSlot.Main, SUPPLIER_ID)
                    .withSlot(SupplierSlot.Main) { it.copy(leadDays = "5") }
                    .withBackupAdded()
                    .withSupplier(SupplierSlot.Backup, SUPPLIER_ID)
                    .withSlot(SupplierSlot.Backup) { it.copy(leadDays = "5") }
            }

            form.onSave()
            val state = shop.settle { form.uiState.value }

            assertEquals(
                FieldIssue.Known(FieldProblem.BackupSameAsMain),
                state.issue(ItemField.Supplier(SupplierSlot.Backup)),
                "O fornecedor reserva precisa ser outro.",
            )
        }

    @Test
    fun optionsNeedANameAChoiceAndUniqueLabels() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val form = shop.form()
            shop.settle { }
            fill(form, name = "Banco", price = "100")
            form.onEdit(null) { f ->
                f
                    .withSupplier(SupplierSlot.Main, SUPPLIER_ID)
                    .withSlot(SupplierSlot.Main) { it.copy(leadDays = "5") }
                    .withNewOption()
            }
            val option =
                form.uiState.value.fields.options
                    .single()

            form.onSave()
            assertEquals(
                FieldIssue.Known(FieldProblem.OptionNameMissing),
                shop.settle { form.uiState.value }.issue(ItemField.OptionName(option.key)),
            )
            form.onEdit(null) { f ->
                f
                    .withOption(option.key) { it.copy(name = "Cor") }
                    .withChoice(option.key, 1) { it.copy(label = "Preta") }
                    .withNewChoice(option.key)
                    .withChoice(option.key, 2) { it.copy(label = "PRETA") }
            }
            form.onSave()

            assertEquals(
                FieldIssue.Known(FieldProblem.ChoiceDuplicate),
                shop.settle { form.uiState.value }.issue(ItemField.ChoiceLabel(option.key, 2)),
            )
        }

    @Test
    fun theServersWordsAppearNextToTheirFields() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val items = RecordingItems(shop.catalog)
            val form = shop.form(items = items)
            shop.settle { }
            fill(form, name = "Banco", price = "100")
            form.onEdit(null) { f ->
                f.withSupplier(SupplierSlot.Main, SUPPLIER_ID).withSlot(SupplierSlot.Main) { it.copy(leadDays = "5") }
            }
            items.refuse =
                Failure.InvalidInput(
                    listOf(
                        InputIssue("mainSupplier.leadDays", "Use um prazo de 0 a 365 dias."),
                        InputIssue("name", "Já existe um item com este nome."),
                    ),
                )

            form.onSave()
            val state = shop.settle { form.uiState.value }

            assertEquals(
                FieldIssue.Server("Use um prazo de 0 a 365 dias."),
                state.issue(ItemField.LeadDays(SupplierSlot.Main)),
            )
            assertEquals(FieldIssue.Server("Já existe um item com este nome."), state.issue(ItemField.Name))
            assertEquals(CatalogMessage.CheckFields, state.message)
            assertFalse(state.saving)
        }

    @Test
    fun aRefusalAboutAFieldTheFormHasNoPlaceForShowsAboveIt() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val items = RecordingItems(shop.catalog)
            val form = shop.form(items = items)
            shop.settle { }
            fill(form, name = "Banco", price = "100")
            form.onEdit(null) { f ->
                f.withSupplier(SupplierSlot.Main, SUPPLIER_ID).withSlot(SupplierSlot.Main) { it.copy(leadDays = "5") }
            }
            items.refuse = Failure.InvalidInput(listOf(InputIssue("somethingNew", "Recusado pelo servidor.")))

            form.onSave()
            val state = shop.settle { form.uiState.value }

            assertEquals(CatalogMessage.General(FailureMessage.Refused("Recusado pelo servidor.")), state.message)
            assertTrue(state.issues.isEmpty())
        }

    @Test
    fun someoneSavingFirstIsAConflictAndReloadShowsTheirVersion() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val seeded = shop.seedItem("Banco Scrambler")
            val form = shop.form(seeded.id)
            shop.settle { }
            shop.catalog.replaceItem(seeded.id, seeded.version, draft("Banco Scrambler Preto"))

            form.onEdit(ItemField.Name) { it.copy(name = "Meu nome") }
            form.onSave()
            val conflict = shop.settle { form.uiState.value }
            assertEquals(CatalogMessage.Conflict, conflict.message, "Outra pessoa alterou este item…")
            assertTrue(shop.published.isEmpty())

            form.onReload()
            val reloaded = shop.settle { form.uiState.value }

            assertNull(reloaded.message)
            assertEquals("Banco Scrambler Preto", reloaded.fields.name)
            assertEquals(seeded.version + 1, reloaded.version)
        }

    @Test
    fun anItemInAKitCannotBeArchivedAndTheMessageNamesTheKit() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val seeded = shop.seedItem("Banco Scrambler")
            shop.catalog.createKit(kitHolding(seeded.id))
            val form = shop.form(seeded.id)
            shop.settle { }

            form.onArchive()
            val state = shop.settle { form.uiState.value }

            assertEquals(CatalogMessage.ItemInKit(listOf("Kit Scrambler")), state.message)
            assertFalse(state.saving)
            assertTrue(shop.published.isEmpty())
            assertFalse(
                shop.storedItem
                    .archived,
            )
        }

    @Test
    fun archivingAndRestoringLeaveTheFormAndTellTheList() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val seeded = shop.seedItem("Banco Scrambler")
            val form = shop.form(seeded.id)
            shop.settle { }

            form.effects.test {
                form.onArchive()
                shop.settle { }
                assertEquals(ItemFormEffect.Close, awaitItem())
            }
            assertTrue(
                shop.storedItem
                    .archived,
            )

            val again = shop.form(seeded.id)
            assertTrue(shop.settle { again.uiState.value }.archived, "Restaurar shows in place of Arquivar")
            again.onRestore()
            shop.settle { }

            assertFalse(
                shop.storedItem
                    .archived,
            )
            assertEquals(
                listOf<CatalogChange>(
                    CatalogChange.ItemArchived("Banco Scrambler"),
                    CatalogChange.ItemRestored("Banco Scrambler"),
                ),
                shop.published,
            )
        }

    @Test
    fun aViewerReadsTheItemAndChangesNothing() =
        runTest(main) {
            val shop = Shop(this, Who.Viewer)
            val seeded = shop.seedItem("Banco Scrambler")
            // A viewer needs neither the store's settings nor its supplier list: reading them must not be tried.
            shop.settings.failure = Failure.Forbidden()
            val items = RecordingItems(shop.catalog)
            val form = shop.form(seeded.id, items)

            val state = shop.settle { form.uiState.value }
            assertFalse(state.canWrite)
            assertFalse(state.editable)
            assertNull(state.loadMessage)
            assertEquals("Banco Scrambler", state.item?.name)
            assertTrue(state.suppliers.isEmpty())

            form.onEdit(ItemField.Name) { it.copy(name = "Outro") }
            form.onSave()
            form.onArchive()
            form.onAddPhoto(PhotoSource.Gallery)
            shop.settle { }

            assertEquals("Banco Scrambler", form.uiState.value.fields.name)
            assertTrue(items.drafts.isEmpty())
            assertEquals(0, shop.picker.galleryOpened)
            assertFalse(
                shop.storedItem
                    .archived,
            )
        }

    @Test
    fun anItemThatCannotBeReadSaysWhyAndReloadsOnRequest() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val seeded = shop.seedItem("Banco Scrambler")
            shop.state.failure = Failure.Offline()
            val form = shop.form(seeded.id)

            val failed = shop.settle { form.uiState.value }
            assertEquals(CatalogMessage.General(FailureMessage.Offline), failed.loadMessage)
            assertFalse(failed.loading)
            assertFalse(failed.editable)

            shop.state.failure = null
            form.onReload()
            val loaded = shop.settle { form.uiState.value }

            assertNull(loaded.loadMessage)
            assertEquals("Banco Scrambler", loaded.fields.name)
        }

    @Test
    fun anItemReadFromTheDevicesCopySaysWhenItWasSaved() =
        runTest(main) {
            val shop = Shop(this, Who.Viewer)
            val seeded = shop.seedItem("Banco Scrambler")
            shop.state.offlineSince = SAVED_AT

            val created = shop.form(seeded.id)
            val state = shop.settle { created.uiState.value }

            assertEquals(SAVED_AT, state.offlineSince)
            assertEquals("Banco Scrambler", state.item?.name)
        }

    private fun assertNotNullIssue(
        form: ItemFormViewModel,
        field: ItemField,
    ) = assertIs<FieldIssue>(form.uiState.value.issue(field))

    private fun kitHolding(itemId: String) =
        KitDraft(
            name = "Kit Scrambler",
            description = "",
            priceCents = 100_000,
            swapDiscount = SwapDiscount.Keep,
            assemblyDays = null,
            safetyMarginDays = 2,
            items = listOf(KitItemDraft(itemId, quantity = 1, swappable = false, alternativeItemIds = emptyList())),
            attributeValueIds = emptyList(),
            published = false,
        )
}
