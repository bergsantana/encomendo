package br.com.encomendo.feature.catalogo

import br.com.encomendo.core.domain.catalog.PickedPhoto
import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.platform.PhotoPicker
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
class ItemFormPhotosTest {
    private val main = StandardTestDispatcher()

    @BeforeTest
    fun useTestMain() = Dispatchers.setMain(main)

    @AfterTest
    fun restoreMain() = Dispatchers.resetMain()

    @Test
    fun aPickedPhotoIsUploadedAndTheSecondOneCanBecomeTheCover() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val seeded = shop.seedItem("Banco Scrambler")
            val form = shop.form(seeded.id)
            shop.settle { }
            shop.picker.gallery += jpeg()
            shop.picker.camera += jpeg()

            form.onAddPhoto(PhotoSource.Gallery)
            shop.settle { }
            form.onAddPhoto(PhotoSource.Camera)
            val two = shop.settle { form.uiState.value }
            assertEquals(2, two.photos.photos.size)
            assertEquals(1, shop.picker.cameraOpened)

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
            assertEquals(listOf(0, 1), ordered.photos.photos.map { it.position })
            assertFalse(ordered.photos.busy)
            assertNull(ordered.photos.message)
        }

    @Test
    fun aPhotoCanBeRemoved() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val seeded = shop.seedItem("Banco Scrambler")
            val form = shop.form(seeded.id)
            shop.settle { }
            shop.picker.gallery += jpeg()
            form.onAddPhoto(PhotoSource.Gallery)
            val photo =
                shop.settle {
                    form.uiState.value.photos.photos
                        .single()
                }

            form.onRemovePhoto(photo)
            shop.settle { }

            val stored =
                shop.storedItem
                    .photos
            assertTrue(
                form.uiState.value.photos.photos
                    .isEmpty(),
            )
            assertTrue(stored.isEmpty())
        }

    @Test
    fun backingOutOfThePickerChangesNothing() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val seeded = shop.seedItem("Banco Scrambler")
            val form = shop.form(seeded.id)
            shop.settle { }

            form.onAddPhoto(PhotoSource.Gallery)
            val state = shop.settle { form.uiState.value }

            assertEquals(1, shop.picker.galleryOpened)
            assertTrue(state.photos.photos.isEmpty())
            assertNull(state.photos.message)
            assertFalse(state.photos.busy)
        }

    @Test
    fun aPhotoOfTheWrongTypeIsRefusedWithTheTypeWording() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val seeded = shop.seedItem("Banco Scrambler")
            val form = shop.form(seeded.id)
            shop.settle { }
            shop.picker.gallery += PickedPhoto(ByteArray(10), "image/gif")

            form.onAddPhoto(PhotoSource.Gallery)
            val state = shop.settle { form.uiState.value }

            assertEquals(
                CatalogMessage.PhotoInvalid,
                state.photos.message,
                "Use uma foto JPEG, PNG ou WebP de até 10 MB.",
            )
            assertTrue(state.photos.photos.isEmpty())
        }

    @Test
    fun aPhotoOver10MbIsRefusedWithTheSizeWording() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val seeded = shop.seedItem("Banco Scrambler")
            val tooBig =
                object : PhotoPicker {
                    override suspend fun pickFromGallery(): PickedPhoto? = throw Failure.PhotoInvalid()

                    override suspend fun takePhoto(): PickedPhoto? = null
                }
            val form = shop.form(seeded.id, picker = tooBig)
            shop.settle { }

            form.onAddPhoto(PhotoSource.Gallery)
            val state = shop.settle { form.uiState.value }

            assertEquals(CatalogMessage.PhotoTooBig, state.photos.message, "Esta foto passa de 10 MB.")
            assertFalse(state.photos.busy)
        }

    @Test
    fun theNinthPhotoIsRefusedWithoutOpeningThePicker() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val seeded = shop.seedItem("Banco Scrambler")
            val form = shop.form(seeded.id)
            shop.settle { }
            repeat(8) { shop.picker.gallery += jpeg() }
            repeat(8) {
                form.onAddPhoto(PhotoSource.Gallery)
                shop.settle { }
            }
            assertEquals(8, form.uiState.value.photos.photos.size)

            form.onAddPhoto(PhotoSource.Gallery)
            val state = shop.settle { form.uiState.value }

            assertEquals(CatalogMessage.PhotoLimit, state.photos.message, "Cada item pode ter até 8 fotos.")
            assertEquals(8, shop.picker.galleryOpened)
        }

    @Test
    fun anUploadThatNeverReachedStorageSaysSoAndKeepsThePhotos() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val seeded = shop.seedItem("Banco Scrambler")
            val form = shop.form(seeded.id)
            shop.settle { }
            shop.picker.gallery += jpeg()
            shop.state.failure = Failure.PhotoNotUploaded()

            form.onAddPhoto(PhotoSource.Gallery)
            val state = shop.settle { form.uiState.value }

            assertEquals(
                CatalogMessage.PhotoNotUploaded,
                state.photos.message,
                "A foto não foi enviada. Tente de novo.",
            )
            assertFalse(state.photos.busy)
        }

    @Test
    fun aNewItemHasNoPhotosUntilItIsSaved() =
        runTest(main) {
            val shop = Shop(this, Who.Admin)
            val form = shop.form()
            shop.settle { }

            form.onAddPhoto(PhotoSource.Gallery)
            shop.settle { }

            assertEquals(0, shop.picker.galleryOpened, "Salve o item para adicionar fotos.")
        }
}
