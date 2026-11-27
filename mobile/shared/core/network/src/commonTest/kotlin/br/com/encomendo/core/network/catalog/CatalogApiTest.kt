package br.com.encomendo.core.network.catalog

import br.com.encomendo.core.network.ApiError
import br.com.encomendo.core.network.ApiErrorCode
import br.com.encomendo.core.network.respondJson
import br.com.encomendo.core.network.respondProblem
import io.ktor.client.engine.mock.respond
import io.ktor.client.request.HttpRequestData
import io.ktor.http.HttpHeaders
import io.ktor.http.HttpMethod
import io.ktor.http.HttpStatusCode
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

class CatalogApiTest {
    private val requests = mutableListOf<HttpRequestData>()
    private val categoryJson = """{"id": "$ITEM_ID", "name": "Bancos", "position": 0}"""
    private val photosJson = """{"items": [$PHOTO_JSON]}"""

    private fun api(
        body: String = "{}",
        status: HttpStatusCode = HttpStatusCode.OK,
    ): CatalogApis =
        catalogApi { request ->
            requests += request
            if (body.isEmpty()) respond("", status) else respondJson(body, status)
        }

    private fun assertCall(
        method: HttpMethod,
        path: String,
    ): HttpRequestData {
        val request = requests.single()
        assertEquals(method, request.method)
        assertEquals(path, request.url.encodedPath)
        assertEquals("Bearer access-alta-rpm", request.headers[HttpHeaders.Authorization])
        return request
    }

    @Test
    fun readsTheCategories() =
        runTest {
            val list = api("""{"items": [$categoryJson]}""").categories.categories()

            assertCall(HttpMethod.Get, "/api/v1/admin/catalog/categories")
            assertEquals("Bancos", list.items.single().name)
        }

    @Test
    fun addsRenamesAndDeletesACategory() =
        runTest {
            api(categoryJson, HttpStatusCode.Created).categories.createCategory("Bancos")
            assertEquals("""{"name":"Bancos"}""", requests.single().bodyText())
            assertCall(HttpMethod.Post, "/api/v1/admin/catalog/categories")

            requests.clear()
            api(categoryJson).categories.updateCategory(ITEM_ID, name = "Selins", position = null)
            // Only what was asked: a null name is refused by the API.
            assertEquals("""{"name":"Selins"}""", requests.single().bodyText())
            assertCall(HttpMethod.Patch, "/api/v1/admin/catalog/categories/$ITEM_ID")

            requests.clear()
            api(categoryJson).categories.updateCategory(ITEM_ID, name = null, position = 2)
            assertEquals("""{"position":2}""", requests.single().bodyText())

            requests.clear()
            api("", HttpStatusCode.NoContent).categories.deleteCategory(ITEM_ID)
            assertCall(HttpMethod.Delete, "/api/v1/admin/catalog/categories/$ITEM_ID")
        }

    @Test
    fun readsAndWritesTheItemFields() =
        runTest {
            val attribute =
                """{"id": "$ITEM_ID", "name": "Serve em", "position": 0,
                    "values": [{"id": "$KIT_ID", "label": "CB 500X", "usageCount": 4}]}"""
            val fields = api("""{"items": [$attribute]}""").categories.attributes()
            assertCall(HttpMethod.Get, "/api/v1/admin/catalog/attributes")
            assertEquals(
                4,
                fields.items
                    .single()
                    .values
                    .single()
                    .usageCount,
            )

            requests.clear()
            val draft =
                AttributeWriteDto(
                    "Serve em",
                    listOf(AttributeValueWriteDto("CB 500X", id = KIT_ID), AttributeValueWriteDto("XRE 300")),
                )
            api(attribute, HttpStatusCode.Created).categories.createAttribute(draft)
            assertCall(HttpMethod.Post, "/api/v1/admin/catalog/attributes")
            // A kept value sends its id; a new one sends none.
            assertEquals(
                """{"name":"Serve em","values":[{"label":"CB 500X","id":"$KIT_ID"},{"label":"XRE 300"}]}""",
                requests.single().bodyText(),
            )

            requests.clear()
            api(attribute).categories.replaceAttribute(ITEM_ID, draft)
            assertCall(HttpMethod.Put, "/api/v1/admin/catalog/attributes/$ITEM_ID")

            requests.clear()
            api("", HttpStatusCode.NoContent).categories.deleteAttribute(ITEM_ID)
            assertCall(HttpMethod.Delete, "/api/v1/admin/catalog/attributes/$ITEM_ID")
        }

    @Test
    fun listsItemsWithTheirFilters() =
        runTest {
            val page =
                api("""{"items": [${itemSummaryJson()}], "nextCursor": "c2"}""")
                    .items
                    .list(query = "banco", categoryId = KIT_ID, includeArchived = true, cursor = "c1", limit = 20)

            val request = assertCall(HttpMethod.Get, "/api/v1/admin/catalog/items")
            assertEquals("banco", request.url.parameters["q"])
            assertEquals(KIT_ID, request.url.parameters["categoryId"])
            assertEquals("true", request.url.parameters["includeArchived"])
            assertEquals("c1", request.url.parameters["cursor"])
            assertEquals("20", request.url.parameters["limit"])
            assertEquals("c2", page.nextCursor)
            assertEquals(
                PHOTO_ID,
                page.items
                    .single()
                    .cover
                    ?.id,
            )
        }

    @Test
    fun anUnfilteredFirstPageSendsOnlyTheLimit() =
        runTest {
            api("""{"items": []}""").items.list(null, null, false, null, 20)

            val parameters = requests.single().url.parameters
            assertEquals(setOf("limit"), parameters.names())
        }

    @Test
    fun readsOneItemAndSavesChangesToIt() =
        runTest {
            api(itemJson()).items.get(ITEM_ID)
            assertCall(HttpMethod.Get, "/api/v1/admin/catalog/items/$ITEM_ID")

            requests.clear()
            api(itemJson(), HttpStatusCode.Created).items.create(writeItem())
            assertCall(HttpMethod.Post, "/api/v1/admin/catalog/items")

            requests.clear()
            api(itemJson()).items.replace(ITEM_ID, writeItem(version = 3))
            assertCall(HttpMethod.Put, "/api/v1/admin/catalog/items/$ITEM_ID")
            assertTrue(""""version":3""" in requests.single().bodyText())
        }

    @Test
    fun publishesArchivesAndRestoresAnItem() =
        runTest {
            api(itemJson()).items.setPublished(ITEM_ID, published = true, version = 3)
            assertCall(HttpMethod.Put, "/api/v1/admin/catalog/items/$ITEM_ID/published")
            assertEquals("""{"published":true,"version":3}""", requests.single().bodyText())

            requests.clear()
            api(itemJson()).items.archive(ITEM_ID, version = 3)
            assertCall(HttpMethod.Post, "/api/v1/admin/catalog/items/$ITEM_ID/archive")
            assertEquals("""{"version":3}""", requests.single().bodyText())

            requests.clear()
            api(itemJson()).items.restore(ITEM_ID, version = 4)
            assertCall(HttpMethod.Post, "/api/v1/admin/catalog/items/$ITEM_ID/restore")
        }

    @Test
    fun listsAndReadsKits() =
        runTest {
            val summary =
                """{"id": "$KIT_ID", "name": "Kit", "itemCount": 2, "price": {"cents": 1, "currency": "BRL"},
                    "singleItemsTotal": {"cents": 2, "currency": "BRL"}, "totalDays": 20, "published": false,
                    "archived": false, "cover": null, "version": 1}"""
            val page = api("""{"items": [$summary]}""").kits.list(query = "kit", includeArchived = false, null, 20)
            val request = assertCall(HttpMethod.Get, "/api/v1/admin/catalog/kits")
            assertEquals("kit", request.url.parameters["q"])
            assertNull(request.url.parameters["includeArchived"])
            assertEquals(20, page.items.single().totalDays)

            requests.clear()
            val kit = api(kitJson()).kits.get(KIT_ID)
            assertCall(HttpMethod.Get, "/api/v1/admin/catalog/kits/$KIT_ID")
            assertEquals(20, kit.days.total)
            assertNull(kit.assemblyDays, "null means the store's default")
        }

    @Test
    fun savesPublishesArchivesAndRestoresAKit() =
        runTest {
            val draft = KitWriteDto("Kit", "", 250000, "keep", null, 2, emptyList(), emptyList(), false)
            api(kitJson(), HttpStatusCode.Created).kits.create(draft)
            assertCall(HttpMethod.Post, "/api/v1/admin/catalog/kits")
            // Null days are sent: the contract requires the keys, and null means the store's default.
            assertTrue(""""assemblyDays":null""" in requests.single().bodyText())

            requests.clear()
            api(kitJson()).kits.replace(KIT_ID, draft.copy(version = 1))
            assertCall(HttpMethod.Put, "/api/v1/admin/catalog/kits/$KIT_ID")

            requests.clear()
            api(kitJson()).kits.setPublished(KIT_ID, published = true, version = 1)
            assertCall(HttpMethod.Put, "/api/v1/admin/catalog/kits/$KIT_ID/published")

            requests.clear()
            api(kitJson()).kits.archive(KIT_ID, version = 1)
            assertCall(HttpMethod.Post, "/api/v1/admin/catalog/kits/$KIT_ID/archive")

            requests.clear()
            api(kitJson()).kits.restore(KIT_ID, version = 2)
            assertCall(HttpMethod.Post, "/api/v1/admin/catalog/kits/$KIT_ID/restore")
        }

    @Test
    fun asksWhereToUploadAPhoto() =
        runTest {
            val upload =
                api(
                    """{"key": "t/x/items/$ITEM_ID/$PHOTO_ID", "uploadUrl": "https://files.test/put?sig=1",
                        "method": "PUT", "headers": {"Content-Type": "image/jpeg"},
                        "expiresAt": "2026-11-27T13:00:00Z"}""",
                    HttpStatusCode.Created,
                ).photos.requestUpload(PhotoOwnerType.Item, ITEM_ID, "image/jpeg")

            assertCall(HttpMethod.Post, "/api/v1/admin/catalog/photo-uploads")
            assertEquals(
                """{"ownerType":"item","ownerId":"$ITEM_ID","contentType":"image/jpeg"}""",
                requests.single().bodyText(),
            )
            assertEquals(mapOf("Content-Type" to "image/jpeg"), upload.headers)
        }

    @Test
    fun confirmsAPhotoOnAnItemOrAKit() =
        runTest {
            val photo = PHOTO_JSON
            api(photo, HttpStatusCode.Created).photos.confirm(PhotoOwnerType.Item, ITEM_ID, "k1")
            assertCall(HttpMethod.Post, "/api/v1/admin/catalog/items/$ITEM_ID/photos")
            assertEquals("""{"key":"k1"}""", requests.single().bodyText())

            // A key confirmed before answers 200 with the photo it recorded: success too.
            requests.clear()
            val again = api(photo, HttpStatusCode.OK).photos.confirm(PhotoOwnerType.Kit, KIT_ID, "k1")
            assertCall(HttpMethod.Post, "/api/v1/admin/catalog/kits/$KIT_ID/photos")
            assertEquals(PHOTO_ID, again.id)
        }

    @Test
    fun ordersAndRemovesPhotos() =
        runTest {
            api(photosJson).photos.order(PhotoOwnerType.Item, ITEM_ID, listOf(PHOTO_ID))
            assertCall(HttpMethod.Put, "/api/v1/admin/catalog/items/$ITEM_ID/photos/order")
            assertEquals("""{"photoIds":["$PHOTO_ID"]}""", requests.single().bodyText())

            requests.clear()
            api("", HttpStatusCode.NoContent).photos.delete(PhotoOwnerType.Kit, KIT_ID, PHOTO_ID)
            assertCall(HttpMethod.Delete, "/api/v1/admin/catalog/kits/$KIT_ID/photos/$PHOTO_ID")
        }

    @Test
    fun anIdNeverLeavesItsPathSegment() =
        runTest {
            api("", HttpStatusCode.NoContent).categories.deleteCategory("../tenants")

            assertEquals("/api/v1/admin/catalog/categories/..%2Ftenants", requests.single().url.encodedPath)
        }

    @Test
    fun theRefusalsNameTheirRows() =
        runTest {
            val inKit =
                catalogApi {
                    respondProblem(
                        HttpStatusCode.Conflict,
                        """{"title":"Item em kit","status":409,"code":"item_in_kit",
                            "references":[{"id":"$KIT_ID","name":"Kit Scrambler"}]}""",
                    )
                }

            val error = assertFailsWith<ApiError> { inKit.items.archive(ITEM_ID, 3) }

            assertEquals(ApiErrorCode.ItemInKit, error.code)
            assertEquals(listOf("Kit Scrambler"), error.references.map { it.name })
        }

    @Test
    fun aWriteWithoutCostsCarriesNoCostKey() =
        runTest {
            api(itemJson(), HttpStatusCode.Created).items.create(writeItem())

            val body = requests.single().bodyText()

            assertFalse("cost" in body.lowercase(), "an admin's write must not carry a cost key: $body")
            // The keys the contract requires are there even when null.
            assertTrue(""""categoryId":null""" in body, body)
        }

    @Test
    fun anOwnersWriteSendsOnlyTheCostsItSet() =
        runTest {
            suspend fun send(item: ItemWriteDto): String {
                requests.clear()
                api(itemJson()).items.replace(ITEM_ID, item)
                return requests.single().bodyText()
            }

            val set = send(writeItem(mainCost = OwnerFieldDto.Value(41000)))
            val cleared = send(writeItem(mainCost = OwnerFieldDto.Null))
            val production = send(writeItem(productionCost = OwnerFieldDto.Value(1500)))

            assertTrue(""""costCents":41000""" in set, set)
            assertTrue(""""costCents":null""" in cleared, cleared)
            assertTrue(""""productionCostCents":1500""" in production, production)
            assertFalse("productionCostCents" in set, "a cost not touched is not sent")
        }

    private fun writeItem(
        version: Int? = null,
        mainCost: OwnerFieldDto<Long> = OwnerFieldDto.Absent,
        productionCost: OwnerFieldDto<Long> = OwnerFieldDto.Absent,
    ) = ItemWriteDto(
        name = "Banco",
        description = "",
        categoryId = null,
        sourcingMode = "to_order",
        priceCents = 89000,
        productionDays = null,
        mainSupplier = SupplierLinkWriteDto(SUPPLIER_ID, leadDays = 15, costCents = mainCost),
        backupSupplier = null,
        attributeValueIds = emptyList(),
        options = emptyList(),
        published = false,
        productionCostCents = productionCost,
        version = version,
    )
}
