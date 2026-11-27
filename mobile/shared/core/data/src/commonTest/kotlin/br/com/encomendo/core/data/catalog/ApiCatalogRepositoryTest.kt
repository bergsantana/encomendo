package br.com.encomendo.core.data.catalog

import br.com.encomendo.core.domain.catalog.CostChange
import br.com.encomendo.core.domain.catalog.ItemFilter
import br.com.encomendo.core.domain.catalog.Margin
import br.com.encomendo.core.domain.catalog.OwnerField
import br.com.encomendo.core.domain.catalog.PhotoOwner
import br.com.encomendo.core.domain.catalog.PhotoOwnerKind
import br.com.encomendo.core.domain.catalog.PickedPhoto
import br.com.encomendo.core.domain.catalog.SourcingMode
import br.com.encomendo.core.domain.catalog.isShown
import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.domain.money.Money
import br.com.encomendo.core.domain.session.Role
import io.ktor.http.HttpHeaders
import io.ktor.http.HttpMethod
import io.ktor.http.HttpStatusCode
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertFalse
import kotlin.test.assertIs
import kotlin.test.assertNull
import kotlin.test.assertTrue

class ApiCatalogRepositoryTest {
    @Test
    fun theOwnerSeesCostsAndMarginsAndNoCostIsNotAHiddenCost() =
        runTest {
            val wired = catalogWiring(Role.Owner)
            wired.api = {
                respondJson(
                    """{"items": [${itemSummaryJson(""","cost": null, "margin": null""")},
                       ${itemSummaryJson(
                        ""","cost": {"cents": 41000, "currency": "BRL"},
                         "margin": {"cents": 48000, "basisPoints": 5393}""",
                    )}]}""",
                )
            }

            val rows =
                wired.catalog
                    .itemsPage(ItemFilter(), cursor = null, limit = 20)
                    .value.items

            assertEquals(OwnerField.Shown(null), rows[0].cost)
            assertEquals(OwnerField.Shown(null), rows[0].margin)
            assertEquals(OwnerField.Shown(Money(41000, "BRL")), rows[1].cost)
            assertEquals(OwnerField.Shown(Margin(48000, 5393)), rows[1].margin)
        }

    @Test
    fun anAdminGetsNoCostColumnAtAll() =
        runTest {
            val wired = catalogWiring(Role.Admin)
            wired.api = { respondJson(itemJson()) }

            val item = wired.catalog.item(ITEM_ID).value

            assertEquals(OwnerField.Hidden, item.cost)
            assertEquals(OwnerField.Hidden, item.margin)
            assertEquals(OwnerField.Hidden, item.productionCost)
            assertFalse(item.mainSupplier?.cost?.isShown ?: true)
        }

    @Test
    fun anAdminsWriteCarriesNoCostKey() =
        runTest {
            val wired = catalogWiring(Role.Admin)
            wired.api = { respondJson(itemJson(), HttpStatusCode.Created) }

            wired.catalog.createItem(itemDraft())
            wired.api = { respondJson(itemJson()) }
            wired.catalog.replaceItem(ITEM_ID, version = 3, draft = itemDraft())

            for (request in wired.apiRequests) {
                val body = request.bodyText()
                assertFalse("cost" in body.lowercase(), "no cost key in $body")
            }
            assertEquals(listOf(HttpMethod.Post, HttpMethod.Put), wired.apiRequests.map { it.method })
        }

    @Test
    fun theOwnersWriteSendsTheCostItSetAndClearsTheOneItCleared() =
        runTest {
            val wired = catalogWiring(Role.Owner)
            wired.api = { respondJson(itemJson()) }

            wired.catalog.replaceItem(ITEM_ID, 3, itemDraft(mainCost = CostChange.Set(41000)))
            wired.catalog.replaceItem(ITEM_ID, 3, itemDraft(mainCost = CostChange.Clear))

            assertTrue(""""costCents":41000""" in wired.apiRequests[0].bodyText())
            assertTrue(""""costCents":null""" in wired.apiRequests[1].bodyText())
        }

    @Test
    fun theAnswerMapsToTheDomain() =
        runTest {
            val wired = catalogWiring()
            wired.api = { respondJson(itemJson()) }

            val item = wired.catalog.item(ITEM_ID).value

            assertEquals(SourcingMode.ToOrder, item.sourcingMode)
            assertEquals(Money(89000, "BRL"), item.price)
            assertEquals(15, item.readyDays)
            assertEquals("Couros Paulista", item.mainSupplier?.supplierName)
            assertEquals(PHOTO_ID, item.photos.single().id)
            assertNull(item.categoryId)
        }

    @Test
    fun anAnswerWithAnUnknownSourcingModeIsUnexpected() =
        runTest {
            val wired = catalogWiring()
            wired.api = { respondJson(itemJson().replace("to_order", "teleport")) }

            assertFailsWith<Failure.Unexpected> { wired.catalog.item(ITEM_ID) }
        }

    @Test
    fun theProblemCodesBecomeFailures() =
        runTest {
            val wired = catalogWiring()

            suspend fun failure(
                status: Int,
                code: String,
                extra: String = "",
                call: suspend () -> Unit = { wired.catalog.archiveItem(ITEM_ID, 3) },
            ): Failure {
                wired.api = { respondProblem(status, code, extra) }
                return assertFailsWith<Failure> { call() }
            }

            val taken = failure(409, "name_taken") { wired.catalog.createCategory("Bancos") }
            assertEquals("name", assertIs<Failure.AlreadyTaken>(taken).field)
            assertIs<Failure.VersionConflict>(failure(409, "version_conflict"))
            assertIs<Failure.CategoryInUse>(failure(409, "category_in_use") { wired.catalog.deleteCategory(ITEM_ID) })
            assertIs<Failure.PhotoLimit>(failure(409, "photo_limit") { upload(wired) })
            assertIs<Failure.PhotoInvalid>(failure(400, "photo_invalid") { upload(wired) })
            assertIs<Failure.PhotoNotUploaded>(failure(400, "photo_not_uploaded") { upload(wired) })

            val inKit =
                failure(409, "item_in_kit", ""","references":[{"id":"$KIT_ID","name":"Kit Scrambler"}]""")
            assertEquals(listOf("Kit Scrambler"), assertIs<Failure.ItemInKit>(inKit).kits.map { it.name })

            val archived =
                failure(409, "item_archived", ""","references":[{"id":"$ITEM_ID","name":"Banco"}]""") {
                    wired.catalog.createKit(kitDraft())
                }
            assertEquals(listOf("Banco"), assertIs<Failure.ItemArchived>(archived).items.map { it.name })
        }

    @Test
    fun aReadWithNoSignalIsServedFromTheLastCopy() =
        runTest {
            val wired = catalogWiring(Role.Owner)
            wired.api = { respondJson(itemPageJson(""","cost": null, "margin": null""")) }
            wired.clock.epochMillis = 5_000
            val online = wired.catalog.itemsPage(ItemFilter(query = "banco"), cursor = null, limit = 20)
            assertFalse(online.fromCache)

            wired.api = { noSignal() }
            val offline = wired.catalog.itemsPage(ItemFilter(query = "banco"), cursor = null, limit = 20)

            assertTrue(offline.fromCache)
            assertEquals(5_000, offline.cachedAtEpochMillis)
            assertEquals(online.value, offline.value, "the same rows, with the owner's null cost kept as null")
            assertEquals("c2", offline.value.nextCursor)
        }

    @Test
    fun aDetailReadWithNoSignalIsServedFromTheLastCopy() =
        runTest {
            val wired = catalogWiring()
            wired.api = { respondJson(itemJson()) }
            wired.catalog.item(ITEM_ID)

            wired.api = { noSignal() }
            val offline = wired.catalog.item(ITEM_ID)

            assertTrue(offline.fromCache)
            assertEquals(ITEM_ID, offline.value.id)
        }

    @Test
    fun aSavedItemOpensOfflineAsSaved() =
        runTest {
            val wired = catalogWiring()
            wired.api = { respondJson(itemJson().replace("Banco de couro", "Banco novo"), HttpStatusCode.Created) }
            wired.catalog.createItem(itemDraft())

            wired.api = { noSignal() }

            assertEquals(
                "Banco novo Scrambler",
                wired.catalog
                    .item(ITEM_ID)
                    .value.name,
            )
        }

    @Test
    fun aListNeverReadIsOfflineNotEmpty() =
        runTest {
            val wired = catalogWiring()
            wired.api = { noSignal() }

            assertFailsWith<Failure.Offline> { wired.catalog.itemsPage(ItemFilter(), cursor = null, limit = 20) }
        }

    @Test
    fun anotherFilterIsAnotherList() =
        runTest {
            val wired = catalogWiring()
            wired.api = { respondJson(itemPageJson()) }
            wired.catalog.itemsPage(ItemFilter(query = "banco"), cursor = null, limit = 20)

            wired.api = { noSignal() }

            assertFailsWith<Failure.Offline> { wired.catalog.itemsPage(ItemFilter(query = "selim"), null, 20) }
            assertFailsWith<Failure.Offline> {
                wired.catalog.itemsPage(ItemFilter(query = "banco", includeArchived = true), null, 20)
            }
        }

    @Test
    fun aFailureOtherThanNoSignalIsNeverAnsweredFromTheCache() =
        runTest {
            val wired = catalogWiring()
            wired.api = { respondJson(itemJson()) }
            wired.catalog.item(ITEM_ID)

            wired.api = { respondProblem(403, "forbidden") }

            assertFailsWith<Failure.Forbidden> { wired.catalog.item(ITEM_ID) }
        }

    @Test
    fun aDemotedOwnerNeverReadsTheOwnersCachedCosts() =
        runTest {
            val wired = catalogWiring(Role.Owner)
            wired.api = { respondJson(itemJson(""","cost": {"cents": 41000, "currency": "BRL"}""")) }
            wired.catalog.item(ITEM_ID)

            // Same store, now an admin, and no signal: the owner's copy must not come back.
            wired.sessions.state.value =
                wired.sessions.state.value
                    ?.copy(role = Role.Admin)
            wired.api = { noSignal() }

            assertFailsWith<Failure.Offline> { wired.catalog.item(ITEM_ID) }
            assertTrue(wired.cache.details.isEmpty(), "the owner's copy is dropped from the device")
        }

    @Test
    fun anAdminsCopyHasNoCostFieldsToLeak() =
        runTest {
            val wired = catalogWiring(Role.Admin)
            wired.api = { respondJson(itemJson()) }
            wired.catalog.item(ITEM_ID)

            val saved =
                wired.cache.details.values
                    .single()
                    .payload

            assertFalse("cost" in saved.lowercase(), "an admin's cached item has no cost key: $saved")
        }

    @Test
    fun aStoreNeverReadsAnotherStoresCopy() =
        runTest {
            val wired = catalogWiring()
            wired.api = { respondJson(itemJson()) }
            wired.catalog.item(ITEM_ID)

            val other =
                wired.sessions.state.value?.copy(
                    tenant =
                        wired.sessions.state.value
                            ?.tenant
                            ?.copy(id = "other"),
                )
            wired.sessions.state.value = other
            wired.api = { noSignal() }

            assertFailsWith<Failure.Offline> { wired.catalog.item(ITEM_ID) }
        }

    @Test
    fun aCopyThatCannotBeReadIsAMissNotACrash() =
        runTest {
            val wired = catalogWiring()
            wired.api = { respondJson(itemJson()) }
            wired.catalog.item(ITEM_ID)
            val key =
                wired.cache.details.keys
                    .single()
            wired.cache.details[key] =
                wired.cache.details
                    .getValue(key)
                    .copy(payload = "{not json")

            wired.api = { noSignal() }

            assertFailsWith<Failure.Offline> { wired.catalog.item(ITEM_ID) }
        }

    @Test
    fun aPhotoIsUploadedInThreeStepsAndTheStorageHostSeesNoBearer() =
        runTest {
            val wired = catalogWiring()
            wired.api = { request ->
                when (request.url.encodedPath) {
                    "/api/v1/admin/catalog/photo-uploads" ->
                        respondJson(
                            """{"key": "$PHOTO_KEY", "uploadUrl": "https://files.test/put?sig=1", "method": "PUT",
                                "headers": {"Content-Type": "image/jpeg"}, "expiresAt": "2026-11-27T13:00:00Z"}""",
                            HttpStatusCode.Created,
                        )
                    else -> respondJson(PHOTO_JSON, HttpStatusCode.Created)
                }
            }

            val photo = upload(wired)

            assertEquals(PHOTO_ID, photo.id)
            val sentToApi = wired.apiRequests.map { it.url.encodedPath }
            assertEquals(
                listOf("/api/v1/admin/catalog/photo-uploads", "/api/v1/admin/catalog/items/$ITEM_ID/photos"),
                sentToApi,
            )
            assertEquals(listOf(true, true), wired.apiRequests.map { it.headers[HttpHeaders.Authorization] != null })
            val put = wired.storageRequests.single()
            assertEquals(HttpMethod.Put, put.method)
            assertEquals("files.test", put.url.host)
            assertNull(put.headers[HttpHeaders.Authorization], "the storage host never sees the bearer token")
            assertTrue(PHOTO_KEY in wired.apiRequests.last().bodyText(), "the confirm sends the key it was given")
        }

    @Test
    fun aConfirmThatAnswers200CountsToo() =
        runTest {
            val wired = catalogWiring()
            wired.api = { request ->
                if (request.url.encodedPath.endsWith("photo-uploads")) {
                    respondJson(
                        """{"key": "$PHOTO_KEY", "uploadUrl": "https://files.test/put", "method": "PUT",
                            "headers": {}, "expiresAt": "2026-11-27T13:00:00Z"}""",
                        HttpStatusCode.Created,
                    )
                } else {
                    respondJson(PHOTO_JSON, HttpStatusCode.OK)
                }
            }

            assertEquals(PHOTO_ID, upload(wired).id)
        }

    @Test
    fun aRefusedUploadIsNotConfirmed() =
        runTest {
            val wired = catalogWiring()
            wired.api = {
                respondJson(
                    """{"key": "$PHOTO_KEY", "uploadUrl": "https://files.test/put", "method": "PUT",
                        "headers": {}, "expiresAt": "2026-11-27T13:00:00Z"}""",
                    HttpStatusCode.Created,
                )
            }
            wired.storage = { respondProblem(403, "forbidden") }

            assertFailsWith<Failure.PhotoNotUploaded> { upload(wired) }

            assertEquals(1, wired.apiRequests.size, "no confirm for a photo that never arrived")
        }

    @Test
    fun aFileTheApiWouldRefuseNeverLeavesTheDevice() =
        runTest {
            val wired = catalogWiring()

            assertFailsWith<Failure.PhotoInvalid> {
                wired.catalog.uploadPhoto(
                    PhotoOwner(PhotoOwnerKind.Item, ITEM_ID),
                    PickedPhoto(byteArrayOf(1), "image/heic"),
                )
            }

            assertTrue(wired.apiRequests.isEmpty())
            assertTrue(wired.storageRequests.isEmpty())
        }

    @Test
    fun noSignalOnAWriteIsOfflineAndNothingIsQueued() =
        runTest {
            val wired = catalogWiring()
            wired.api = { noSignal() }

            assertFailsWith<Failure.Offline> { wired.catalog.createItem(itemDraft()) }
        }

    private suspend fun upload(wired: CatalogWiring) =
        wired.catalog.uploadPhoto(
            PhotoOwner(PhotoOwnerKind.Item, ITEM_ID),
            PickedPhoto(byteArrayOf(1, 2, 3), "image/jpeg"),
        )
}
