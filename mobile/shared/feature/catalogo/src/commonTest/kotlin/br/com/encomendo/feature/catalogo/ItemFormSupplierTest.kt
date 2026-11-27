package br.com.encomendo.feature.catalogo

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

@OptIn(ExperimentalCoroutinesApi::class) // setMain / resetMain
class ItemFormSupplierTest {
    private val main = StandardTestDispatcher()

    @BeforeTest
    fun useTestMain() = Dispatchers.setMain(main)

    @AfterTest
    fun restoreMain() = Dispatchers.resetMain()

    @Test
    fun aNewSupplierIsCreatedAndPickedForTheSlotThatAskedForIt() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val form = shop.form()
            shop.settle { }

            form.onOpenNewSupplier(SupplierSlot.Main)
            form.onNewSupplierName("  Selaria Norte ")
            form.onNewSupplierPhone("(92) 99123-4567")
            form.onAddSupplier()
            val state = shop.settle { form.uiState.value }

            assertNull(state.newSupplier, "the form closes")
            val created = shop.state.suppliers.single { it.name == "Selaria Norte" }
            assertEquals("5592991234567", created.whatsappPhone, "digits only, with the country code")
            assertEquals(created.id, state.fields.main.supplierId)
            assertEquals("Selaria Norte", state.supplierName(created.id))
            assertTrue(state.suppliers.any { it.id == created.id })
        }

    @Test
    fun aNewSuppliersEmptyAndWrongFieldsAreMarkedNotSent() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val form = shop.form()
            shop.settle { }
            form.onOpenNewSupplier(SupplierSlot.Backup)

            form.onAddSupplier()
            val empty = shop.settle { form.uiState.value.newSupplier }
            assertEquals(SupplierProblem.NameMissing, empty?.nameIssue)
            assertEquals(SupplierProblem.PhoneMissing, empty?.phoneIssue)

            form.onNewSupplierName("x".repeat(121))
            form.onNewSupplierPhone("12345")
            form.onAddSupplier()
            val wrong = form.uiState.value.newSupplier
            assertEquals(SupplierProblem.NameTooLong, wrong?.nameIssue)
            assertEquals(SupplierProblem.PhoneInvalid, wrong?.phoneIssue)

            form.onNewSupplierName("Nome\ncom quebra")
            form.onAddSupplier()
            assertEquals(
                SupplierProblem.NameControlCharacters,
                form.uiState.value.newSupplier
                    ?.nameIssue,
            )
            assertEquals(2, shop.state.suppliers.size, "nothing was created")
        }

    @Test
    fun aSupplierNameTakenIsMarkedNextToTheName() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val form = shop.form()
            shop.settle { }
            form.onOpenNewSupplier(SupplierSlot.Main)
            form.onNewSupplierName("couros paulista")
            form.onNewSupplierPhone("11987654321")

            form.onAddSupplier()
            val state = shop.settle { form.uiState.value }

            assertEquals(SupplierProblem.NameTaken, state.newSupplier?.nameIssue)
            assertFalse(state.newSupplier?.saving == true)
        }

    @Test
    fun cancellingNewSupplierKeepsWhatWasTypedInTheItem() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val form = shop.form()
            shop.settle { }
            fill(form, name = "Banco", price = "100")

            form.onOpenNewSupplier(SupplierSlot.Main)
            form.onCloseNewSupplier()

            assertNull(form.uiState.value.newSupplier)
            assertEquals("Banco", form.uiState.value.fields.name)
        }
}
