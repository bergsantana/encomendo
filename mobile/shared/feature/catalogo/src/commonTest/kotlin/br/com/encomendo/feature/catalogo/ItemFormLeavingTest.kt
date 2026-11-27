package br.com.encomendo.feature.catalogo

import app.cash.turbine.test
import br.com.encomendo.core.domain.catalog.CostChange
import br.com.encomendo.core.domain.catalog.Photo
import br.com.encomendo.core.domain.catalog.PhotoOwner
import br.com.encomendo.core.domain.catalog.PhotoRepository
import br.com.encomendo.core.domain.catalog.PickedPhoto
import br.com.encomendo.core.domain.catalog.SourcingMode
import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.domain.supplier.Supplier
import br.com.encomendo.core.domain.supplier.SupplierDraft
import br.com.encomendo.core.domain.supplier.SupplierRepository
import br.com.encomendo.core.ui.failure.FailureMessage
import kotlinx.coroutines.CompletableDeferred
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

/** Photo uploads that wait for the test, so what shows while one is under way can be seen. */
private class GatedPhotos(
    private val delegate: PhotoRepository,
    val gate: CompletableDeferred<Unit>,
) : PhotoRepository by delegate {
    override suspend fun uploadPhoto(
        owner: PhotoOwner,
        photo: PickedPhoto,
    ): Photo {
        gate.await()
        return delegate.uploadPhoto(owner, photo)
    }
}

/** Supplier creation that waits for the test. */
private class GatedSuppliers(
    private val delegate: SupplierRepository,
    val gate: CompletableDeferred<Unit>,
) : SupplierRepository by delegate {
    override suspend fun create(draft: SupplierDraft): Supplier {
        gate.await()
        return delegate.create(draft)
    }
}

/** Leaving the form, what an editor does offline, what the fields remember, and what shows while something saves. */
@OptIn(ExperimentalCoroutinesApi::class) // setMain / resetMain
class ItemFormLeavingTest {
    private val main = StandardTestDispatcher()

    @BeforeTest
    fun useTestMain() = Dispatchers.setMain(main)

    @AfterTest
    fun restoreMain() = Dispatchers.resetMain()

    @Test
    fun cancelWithNothingTypedClosesAtOnce() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val seeded = shop.seedItem("Banco Scrambler")
            val form = shop.form(seeded.id)
            shop.settle { }

            form.effects.test {
                form.onCancel()
                shop.settle { }
                assertEquals(ItemFormEffect.Close, awaitItem())
            }
            assertNull(form.uiState.value.discard)
        }

    @Test
    fun cancelWithEditsAsksAndDiscardingLeaves() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val seeded = shop.seedItem("Banco Scrambler")
            val form = shop.form(seeded.id)
            shop.settle { }
            form.onEdit(ItemField.Name) { it.copy(name = "Outro nome") }
            assertTrue(form.uiState.value.dirty)

            form.effects.test {
                form.onCancel()
                shop.settle { }
                assertEquals(DiscardAction.Close, form.uiState.value.discard, "Descartar as alterações?")
                expectNoEvents()

                form.onKeepEditing()
                assertNull(form.uiState.value.discard)
                assertEquals("Outro nome", form.uiState.value.fields.name, "Continuar editando")

                form.onCancel()
                form.onDiscard()
                shop.settle { }
                assertEquals(ItemFormEffect.Close, awaitItem())
            }
            assertEquals(
                "Banco Scrambler",
                shop.storedItem
                    .name,
                "Descartar saves nothing",
            )
        }

    @Test
    fun typingBackWhatWasThereIsNotAnEdit() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val seeded = shop.seedItem("Banco Scrambler")
            val form = shop.form(seeded.id)
            shop.settle { }

            form.onEdit(ItemField.Name) { it.copy(name = "Outro") }
            form.onEdit(ItemField.Name) { it.copy(name = "Banco Scrambler") }

            assertFalse(form.uiState.value.dirty)
        }

    @Test
    fun aNewItemWithSomethingTypedAsksToo() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val form = shop.form()
            shop.settle { }
            fill(form, name = "Banco", price = "")

            form.onCancel()

            assertEquals(DiscardAction.Close, form.uiState.value.discard)
        }

    @Test
    fun archivingWithEditsAsksFirstAndNothingIsArchivedMeanwhile() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val seeded = shop.seedItem("Banco Scrambler")
            val form = shop.form(seeded.id)
            shop.settle { }
            form.onEdit(ItemField.Name) { it.copy(name = "Outro nome") }

            form.onArchive()
            shop.settle { }
            assertEquals(DiscardAction.Archive, form.uiState.value.discard)
            assertFalse(
                shop.storedItem
                    .archived,
            )

            form.onDiscard()
            shop.settle { }

            assertTrue(
                shop.storedItem
                    .archived,
            )
            assertEquals(
                "Banco Scrambler",
                shop.storedItem
                    .name,
                "the edits were dropped",
            )
            assertEquals(listOf<CatalogChange>(CatalogChange.ItemArchived("Banco Scrambler")), shop.published)
        }

    @Test
    fun aReadOnlyItemClosesWithFechar() =
        runTest(main) {
            val shop = Shop(this, Who.Viewer)
            val seeded = shop.seedItem("Banco Scrambler")
            val form = shop.form(seeded.id)
            shop.settle { }

            form.effects.test {
                form.onCancel()
                shop.settle { }
                assertEquals(ItemFormEffect.Close, awaitItem())
            }
            assertNull(form.uiState.value.discard)
        }

    @Test
    fun anAdminOfflineWithACachedItemOpensItReadOnlyWithTheBanner() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val seeded = shop.seedItem("Banco Scrambler")
            shop.state.offlineSince = SAVED_AT
            shop.settings.failure = Failure.Offline()
            val form = shop.form(seeded.id)

            val state = shop.settle { form.uiState.value }

            assertFalse(state.canWrite, "it could not save anyway")
            assertFalse(state.editable)
            assertNull(state.loadMessage)
            assertEquals(SAVED_AT, state.offlineSince)
            assertEquals("Banco Scrambler", state.item?.name)
            assertTrue(state.suppliers.isEmpty())

            form.onEdit(ItemField.Name) { it.copy(name = "Outro") }
            form.onSave()
            shop.settle { }
            assertEquals("Banco Scrambler", form.uiState.value.fields.name)
        }

    @Test
    fun anEditorOfflineWithNothingCachedSaysSoInsteadOfOpening() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            shop.settings.failure = Failure.Offline()
            val form = shop.form()

            val state = shop.settle { form.uiState.value }

            assertEquals(CatalogMessage.General(FailureMessage.Offline), state.loadMessage)
            assertFalse(state.editable)
        }

    @Test
    fun choosingSupplierAThenBThenAShowsAsCostAgain() =
        runTest(main) {
            val shop = Shop(this, Who.Owner)
            val seeded = shop.seedItem("Banco Scrambler")
            val items = RecordingItems(shop.catalog)
            val form = shop.form(seeded.id, items)
            val loaded = shop.settle { form.uiState.value }
            assertEquals("40,00", loaded.fields.main.cost)

            form.onEdit(null) { it.withSupplier(SupplierSlot.Main, OTHER_SUPPLIER_ID) }
            val other = form.uiState.value.fields.main
            assertEquals("", other.cost, "a supplier new to the item starts with none")
            assertFalse(other.costTouched)

            form.onEdit(null) { it.withSupplier(SupplierSlot.Main, SUPPLIER_ID) }
            val back = form.uiState.value.fields.main
            assertEquals("40,00", back.cost, "its own stored cost again")
            assertFalse(back.costTouched, "so the save leaves it to the server")

            form.onSave()
            shop.settle { }
            assertEquals(
                CostChange.Keep,
                items.drafts
                    .single()
                    .mainSupplier
                    ?.cost,
            )
        }

    @Test
    fun productionDaysOutsideOneTo365AreADifferentMistakeFromBlank() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val form = shop.form()
            shop.settle { }
            fill(form, name = "Capa", price = "80")
            form.onModeChange(SourcingMode.InHouse)

            form.onEdit(ItemField.ProductionDays) { it.copy(productionDays = "400") }
            form.onSave()
            assertEquals(
                FieldIssue.Known(FieldProblem.ProductionDaysRange),
                form.uiState.value.issue(ItemField.ProductionDays),
                "Use de 1 a 365 dias de produção.",
            )

            form.onEdit(ItemField.ProductionDays) { it.copy(productionDays = "0") }
            form.onSave()
            assertEquals(
                FieldIssue.Known(FieldProblem.ProductionDaysRange),
                form.uiState.value.issue(ItemField.ProductionDays),
            )

            form.onEdit(ItemField.ProductionDays) { it.copy(productionDays = "") }
            form.onSave()
            assertEquals(
                FieldIssue.Known(FieldProblem.ProductionDaysMissing),
                form.uiState.value.issue(ItemField.ProductionDays),
            )
        }

    @Test
    fun anEmptyChoiceSaysItsOwnWords() =
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
            form.onEdit(null) { it.withOption(option.key) { o -> o.copy(name = "Cor") } }

            form.onSave()

            assertEquals(
                FieldIssue.Known(FieldProblem.ChoiceLabelMissing),
                form.uiState.value.issue(ItemField.ChoiceLabel(option.key, 1)),
            )
        }

    @Test
    fun whileAPhotoGoesUpItSaysSoAndEveryPhotoButtonWaits() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val seeded = shop.seedItem("Banco Scrambler")
            val gate = CompletableDeferred<Unit>()
            val form = shop.form(seeded.id, photos = GatedPhotos(shop.catalog, gate))
            shop.settle { }
            shop.picker.gallery += jpeg()

            form.onAddPhoto(PhotoSource.Gallery)
            val uploading = shop.settle { form.uiState.value.photos }

            assertTrue(uploading.uploading, "Enviando a foto…")
            assertTrue(uploading.busy)
            assertFalse(uploading.canChange)

            gate.complete(Unit)
            val done = shop.settle { form.uiState.value.photos }

            assertFalse(done.uploading)
            assertFalse(done.busy)
            assertEquals(1, done.photos.size)
        }

    @Test
    fun newSupplierCannotBeClosedWhileItSaves() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val gate = CompletableDeferred<Unit>()
            val form = shop.form(supplierRepository = GatedSuppliers(shop.suppliers, gate))
            shop.settle { }
            form.onOpenNewSupplier(SupplierSlot.Main)
            form.onNewSupplierName("Selaria Norte")
            form.onNewSupplierPhone("92991234567")

            form.onAddSupplier()
            shop.settle { }
            form.onCloseNewSupplier()
            assertEquals(
                true,
                form.uiState.value.newSupplier
                    ?.saving,
                "the dialog stays while it saves",
            )

            gate.complete(Unit)
            val state = shop.settle { form.uiState.value }

            assertNull(state.newSupplier)
            assertEquals("Selaria Norte", state.supplierName(state.fields.main.supplierId))
        }

    @Test
    fun namesAreJoinedTheWayAPortugueseReaderExpects() {
        assertEquals("A", listOf("A").joinedWithE())
        assertEquals("A e B", listOf("A", "B").joinedWithE())
        assertEquals("A, B e C", listOf("A", "B", "C").joinedWithE())
        assertEquals("", emptyList<String>().joinedWithE())
    }
}
