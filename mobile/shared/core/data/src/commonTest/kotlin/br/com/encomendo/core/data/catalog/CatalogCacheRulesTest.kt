package br.com.encomendo.core.data.catalog

import br.com.encomendo.core.database.CatalogPageEntity
import br.com.encomendo.core.domain.catalog.ItemFilter
import br.com.encomendo.core.domain.catalog.PhotoOwner
import br.com.encomendo.core.domain.catalog.PhotoOwnerKind
import br.com.encomendo.core.domain.catalog.PickedPhoto
import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.domain.session.Role
import io.ktor.http.HttpStatusCode
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertFailsWith
import kotlin.test.assertFalse
import kotlin.test.assertTrue

/**
 * What the offline cache keeps and forgets: costs by what the payload says, rows a write made stale, and a
 * copy that no longer maps.
 */
class CatalogCacheRulesTest {
    @Test
    fun aRowIsACostsRowByItsPayloadNotByTheRoleTheDeviceThinksItHas() =
        runTest {
            // The session still says admin, but the API sent a cost (the user became the owner meanwhile).
            val wired = catalogWiring(Role.Admin)
            wired.api = { respondJson(itemJson(""","cost": {"cents": 5, "currency": "BRL"}""")) }
            wired.catalog.item(ITEM_ID)
            assertTrue(
                wired.cache.details.values
                    .single()
                    .costs,
            )

            wired.api = { noSignal() }

            assertFailsWith<Failure.Offline> { wired.catalog.item(ITEM_ID) }
            assertTrue(wired.cache.details.isEmpty(), "a role that does not see costs deletes the costs rows")
        }

    @Test
    fun sairEmptiesTheCacheOfEveryStoreCostsIncluded() =
        runTest {
            val wired = catalogWiring(Role.Owner)
            wired.api = { respondJson(itemJson(""","cost": {"cents": 5, "currency": "BRL"}""")) }
            wired.catalog.item(ITEM_ID)
            wired.cache.pages[listOf("another-store", "items", "", "")] =
                CatalogPageEntity(
                    "another-store",
                    "items",
                    "",
                    "",
                    costs = true,
                    payload = "{}",
                    cachedAtEpochMillis = 1,
                )
            assertFalse(wired.cache.details.isEmpty())

            CatalogCacheCleaner(wired.cache.daos).clear()

            assertTrue(wired.cache.details.isEmpty())
            assertTrue(wired.cache.pages.isEmpty(), "every store's pages, not only the one signed in")
        }

    @Test
    fun aRowWithNoCostKeyIsNotACostsRowEvenForTheOwner() =
        runTest {
            val wired = catalogWiring(Role.Owner)
            wired.api = { respondJson(itemJson()) }
            wired.catalog.item(ITEM_ID)

            assertFalse(
                wired.cache.details.values
                    .single()
                    .costs,
            )
        }

    @Test
    fun aKeyNestedInAListMakesItACostsPage() =
        runTest {
            val wired = catalogWiring(Role.Owner)
            wired.api = { respondJson(itemPageJson(""","cost": null""")) }

            wired.catalog.itemsPage(ItemFilter(), cursor = null, limit = 20)

            assertTrue(
                wired.cache.pages.values
                    .single()
                    .costs,
            )
        }

    @Test
    fun aWriteForgetsTheListsAndKeepsTheRowAsSaved() =
        runTest {
            val wired = catalogWiring()
            wired.api = { respondJson(itemPageJson()) }
            wired.catalog.itemsPage(ItemFilter(), cursor = null, limit = 20)
            wired.api = { respondJson(itemJson().replace(""""archived": false""", """"archived": true""")) }

            wired.catalog.archiveItem(ITEM_ID, version = 3)

            wired.api = { noSignal() }
            assertFailsWith<Failure.Offline> { wired.catalog.itemsPage(ItemFilter(), cursor = null, limit = 20) }
            assertTrue(
                wired.catalog
                    .item(ITEM_ID)
                    .value.archived,
                "the detail is the row as saved",
            )
        }

    @Test
    fun aPhotoChangeForgetsTheOwnersDetailAndLists() =
        runTest {
            val wired = catalogWiring()
            wired.api = { respondJson(itemJson()) }
            wired.catalog.item(ITEM_ID)
            wired.api = { respondJson(itemPageJson()) }
            wired.catalog.itemsPage(ItemFilter(), cursor = null, limit = 20)
            val owner = PhotoOwner(PhotoOwnerKind.Item, ITEM_ID)

            wired.api = { respondJson("""{"items": []}""") }
            wired.catalog.orderPhotos(owner, emptyList())

            wired.api = { noSignal() }
            assertFailsWith<Failure.Offline> { wired.catalog.item(ITEM_ID) }
            assertFailsWith<Failure.Offline> { wired.catalog.itemsPage(ItemFilter(), cursor = null, limit = 20) }
        }

    @Test
    fun anUploadAndARemovalForgetTheOwnersDetail() =
        runTest {
            val wired = catalogWiring()
            val owner = PhotoOwner(PhotoOwnerKind.Item, ITEM_ID)
            wired.api = { respondJson(itemJson()) }
            wired.catalog.item(ITEM_ID)
            wired.api = { request ->
                if (request.url.encodedPath.endsWith("photo-uploads")) {
                    respondJson(
                        """{"key": "$PHOTO_KEY", "uploadUrl": "https://files.test/put", "method": "PUT",
                            "headers": {}, "expiresAt": "2026-11-27T13:00:00Z"}""",
                        HttpStatusCode.Created,
                    )
                } else {
                    respondJson(PHOTO_JSON, HttpStatusCode.Created)
                }
            }

            wired.catalog.uploadPhoto(owner, PickedPhoto(byteArrayOf(1), "image/jpeg"))

            wired.api = { noSignal() }
            assertFailsWith<Failure.Offline> { wired.catalog.item(ITEM_ID) }

            wired.api = { respondJson(itemJson()) }
            wired.catalog.item(ITEM_ID)
            wired.api = { respondJson("", HttpStatusCode.NoContent) }
            wired.catalog.deletePhoto(owner, PHOTO_ID)
            wired.api = { noSignal() }
            assertFailsWith<Failure.Offline> { wired.catalog.item(ITEM_ID) }
        }

    @Test
    fun aCopyThatNoLongerMapsIsTheOriginalOfflineFailure() =
        runTest {
            val wired = catalogWiring()
            wired.api = { respondJson(itemJson()) }
            wired.catalog.item(ITEM_ID)
            val key =
                wired.cache.details.keys
                    .single()
            val saved = wired.cache.details.getValue(key)
            wired.cache.details[key] = saved.copy(payload = saved.payload.replace("to_order", "teleport"))

            wired.api = { noSignal() }

            assertFailsWith<Failure.Offline> { wired.catalog.item(ITEM_ID) }
        }
}
