package br.com.encomendo.core.data.catalog

import br.com.encomendo.core.domain.catalog.CostChange
import br.com.encomendo.core.domain.catalog.ItemFilter
import br.com.encomendo.core.domain.catalog.OwnerField
import br.com.encomendo.core.domain.catalog.PhotoOwner
import br.com.encomendo.core.domain.catalog.PhotoOwnerKind
import br.com.encomendo.core.domain.catalog.PickedPhoto
import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.domain.supplier.Supplier
import br.com.encomendo.core.domain.supplier.SupplierDraft
import br.com.encomendo.core.testing.FakeCatalogRepository
import br.com.encomendo.core.testing.FakeCatalogState
import br.com.encomendo.core.testing.FakePhotoPicker
import br.com.encomendo.core.testing.FakeSupplierRepository
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertFalse
import kotlin.test.assertIs
import kotlin.test.assertNull
import kotlin.test.assertTrue

/**
 * The fakes the ViewModel tests of steps 3.15 and 3.16 rely on must follow the same rules as the API, or those
 * tests would pass against behavior the server never has. These check the rules the screens meet.
 */
class FakeCatalogRepositoryTest {
    private val state =
        FakeCatalogState().apply {
            suppliers = listOf(Supplier(SUPPLIER_ID, "Couros Paulista", "5511987654321", version = 1))
        }
    private val catalog = FakeCatalogRepository(state)

    @Test
    fun anOwnerSeesTheCostItSetAndAnAdminIsRefusedOne() =
        runTest {
            state.seesCosts = true
            val owners = catalog.createItem(itemDraft(mainCost = CostChange.Set(41000)))
            assertEquals(41000, (owners.cost as OwnerField.Shown).value?.cents)

            state.seesCosts = false
            assertFailsWith<Failure.Forbidden> { catalog.createItem(itemDraft(mainCost = CostChange.Set(1))) }
            val admins = catalog.createItem(itemDraft())
            assertEquals(OwnerField.Hidden, admins.cost)
        }

    @Test
    fun savingKeepsTheCostTheAdminCouldNotSee() =
        runTest {
            state.seesCosts = true
            val created = catalog.createItem(itemDraft(mainCost = CostChange.Set(41000)))

            state.seesCosts = true
            val saved = catalog.replaceItem(created.id, created.version, itemDraft(mainCost = CostChange.Keep))

            assertEquals(created.cost, saved.cost)
        }

    @Test
    fun aStaleVersionIsAConflict() =
        runTest {
            val created = catalog.createItem(itemDraft())
            catalog.replaceItem(created.id, created.version, itemDraft())

            assertFailsWith<Failure.VersionConflict> { catalog.replaceItem(created.id, created.version, itemDraft()) }
        }

    @Test
    fun anItemInAKitCannotBeArchivedUntilTheKitIs() =
        runTest {
            val item = catalog.createItem(itemDraft())
            val kit =
                catalog.createKit(
                    kitDraft().copy(items = listOf(kitDraft().items.single().copy(itemId = item.id))),
                )

            val refused = assertFailsWith<Failure.ItemInKit> { catalog.archiveItem(item.id, item.version) }
            assertEquals(listOf(kit.name), refused.kits.map { it.name })

            val archivedKit = catalog.archiveKit(kit.id, kit.version)
            val archived = catalog.archiveItem(item.id, item.version)
            assertTrue(archived.archived)
            assertFalse(archived.published)
            assertFailsWith<Failure.ItemArchived> { catalog.restoreKit(archivedKit.id, archivedKit.version) }
        }

    @Test
    fun theListHidesArchivedItemsAndPages() =
        runTest {
            val first = catalog.createItem(itemDraft())
            catalog.createItem(itemDraft())
            catalog.archiveItem(first.id, first.version)

            val page = catalog.itemsPage(ItemFilter(), cursor = null, limit = 1)
            val all = catalog.itemsPage(ItemFilter(includeArchived = true), cursor = null, limit = 1)

            assertEquals(1, page.value.items.size)
            assertNull(page.value.nextCursor)
            assertEquals("1", all.value.nextCursor)
            val second = catalog.itemsPage(ItemFilter(includeArchived = true), all.value.nextCursor, 1)
            assertEquals(1, second.value.items.size)
            assertNull(second.value.nextCursor)
        }

    @Test
    fun anOfflineReadSaysWhenTheCopyWasSaved() =
        runTest {
            state.offlineSince = SAVED_AT

            val read = catalog.categories()

            assertTrue(read.fromCache)
            assertEquals(SAVED_AT, read.cachedAtEpochMillis)
        }

    @Test
    fun aFailureStopsEveryCall() =
        runTest {
            state.failure = Failure.Offline()

            assertFailsWith<Failure.Offline> { catalog.categories() }
            assertFailsWith<Failure.Offline> { catalog.createItem(itemDraft()) }
        }

    @Test
    fun aCategoryInUseCannotBeDeleted() =
        runTest {
            val category = catalog.createCategory("Bancos")
            catalog.createItem(itemDraft().copy(categoryId = category.id))

            assertFailsWith<Failure.CategoryInUse> { catalog.deleteCategory(category.id) }
            assertEquals("name", assertFailsWith<Failure.AlreadyTaken> { catalog.createCategory("bancos") }.field)
        }

    @Test
    fun aCategoryMovesToItsPlaceAndAnotherCannotTakeItsName() =
        runTest {
            catalog.createCategory("Bancos")
            catalog.createCategory("Guidões")
            val third = catalog.createCategory("Manoplas")

            catalog.moveCategory(third.id, 0)

            assertEquals(listOf("Manoplas", "Bancos", "Guidões"), catalog.categories().value.map { it.name })
            assertEquals(listOf(0, 1, 2), catalog.categories().value.map { it.position })
            assertEquals(
                "name",
                assertFailsWith<Failure.AlreadyTaken> { catalog.renameCategory(third.id, "bancos") }.field,
            )
            assertEquals("MANOPLAS", catalog.renameCategory(third.id, "MANOPLAS").name, "its own name, in other case")
        }

    @Test
    fun aPhotoIsLastAndTheNinthIsRefused() =
        runTest {
            val item = catalog.createItem(itemDraft())
            val owner = PhotoOwner(PhotoOwnerKind.Item, item.id)
            val jpeg = PickedPhoto(byteArrayOf(1), "image/jpeg")

            repeat(PHOTOS) { assertEquals(it, catalog.uploadPhoto(owner, jpeg).position) }

            assertFailsWith<Failure.PhotoLimit> { catalog.uploadPhoto(owner, jpeg) }
            assertFailsWith<Failure.PhotoInvalid> {
                catalog.uploadPhoto(
                    owner,
                    PickedPhoto(byteArrayOf(1), "image/gif"),
                )
            }
        }

    @Test
    fun suppliersAreUniqueAndVersioned() =
        runTest {
            val suppliers = FakeSupplierRepository(state)
            val created = suppliers.create(SupplierDraft("Metais SP", "5511911112222"))

            assertFailsWith<Failure.AlreadyTaken> { suppliers.create(SupplierDraft("metais sp", "5511911112222")) }
            val renamed =
                suppliers.replace(
                    created.id,
                    created.version,
                    SupplierDraft("Metais ABC", created.whatsappPhone),
                )
            assertEquals(created.version + 1, renamed.version)
            assertFailsWith<Failure.VersionConflict> {
                suppliers.replace(
                    created.id,
                    created.version,
                    SupplierDraft("x", "y"),
                )
            }
            assertIs<Supplier>(suppliers.supplier(created.id).value)
        }

    @Test
    fun theFakePickerAnswersWhatATestQueuedThenBacksOut() =
        runTest {
            val picker = FakePhotoPicker()
            picker.gallery += PickedPhoto(byteArrayOf(1), "image/png")

            assertEquals("image/png", picker.pickFromGallery()?.contentType)
            assertNull(picker.pickFromGallery(), "nothing queued is the user backing out")
            assertNull(picker.takePhoto())
            assertEquals(2, picker.galleryOpened)
            assertEquals(1, picker.cameraOpened)
        }

    private companion object {
        const val SAVED_AT = 1234L
        const val PHOTOS = 8
    }
}
