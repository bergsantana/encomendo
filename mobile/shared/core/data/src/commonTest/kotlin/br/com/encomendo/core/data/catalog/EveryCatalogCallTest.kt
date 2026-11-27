package br.com.encomendo.core.data.catalog

import br.com.encomendo.core.domain.catalog.ItemFilter
import br.com.encomendo.core.domain.catalog.KitFilter
import br.com.encomendo.core.domain.catalog.PhotoOwner
import br.com.encomendo.core.domain.catalog.PhotoOwnerKind
import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.domain.supplier.SupplierDraft
import io.ktor.client.engine.mock.MockRequestHandleScope
import io.ktor.client.engine.mock.respond
import io.ktor.client.request.HttpRequestData
import io.ktor.client.request.HttpResponseData
import io.ktor.http.HttpMethod
import io.ktor.http.HttpStatusCode
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertFalse
import kotlin.test.assertTrue

/**
 * Every operation of the catalog and suppliers contract reaches its route with its method, and a failure it
 * gets back is a `Failure`, never an `ApiError`. One handler answers every route.
 */
class EveryCatalogCallTest {
    private class Call(
        val method: HttpMethod,
        val path: String,
        val run: suspend (CatalogWiring) -> Unit,
    )

    private val item = PhotoOwner(PhotoOwnerKind.Item, ITEM_ID)
    private val kit = PhotoOwner(PhotoOwnerKind.Kit, KIT_ID)
    private val supplier = SupplierDraft("Couros Paulista", "5511987654321")

    private val categories = "$CATALOG/categories/$ITEM_ID"
    private val attributes = "$CATALOG/attributes/$ITEM_ID"
    private val items = "$CATALOG/items/$ITEM_ID"
    private val kits = "$CATALOG/kits/$KIT_ID"

    private val calls =
        listOf(
            Call(HttpMethod.Get, "$CATALOG/categories") { it.catalog.categories() },
            Call(HttpMethod.Post, "$CATALOG/categories") { it.catalog.createCategory("Bancos") },
            Call(HttpMethod.Patch, categories) { it.catalog.renameCategory(ITEM_ID, "Selins") },
            Call(HttpMethod.Patch, categories) { it.catalog.moveCategory(ITEM_ID, POSITION) },
            Call(HttpMethod.Delete, categories) { it.catalog.deleteCategory(ITEM_ID) },
            Call(HttpMethod.Get, "$CATALOG/attributes") { it.catalog.attributes() },
            Call(HttpMethod.Post, "$CATALOG/attributes") { it.catalog.createAttribute(attributeDraft()) },
            Call(HttpMethod.Put, attributes) { it.catalog.replaceAttribute(ITEM_ID, attributeDraft()) },
            Call(HttpMethod.Delete, attributes) { it.catalog.deleteAttribute(ITEM_ID) },
            Call(HttpMethod.Get, "$CATALOG/items") { it.catalog.itemsPage(ItemFilter(), null, LIMIT) },
            Call(HttpMethod.Get, items) { it.catalog.item(ITEM_ID) },
            Call(HttpMethod.Post, "$CATALOG/items") { it.catalog.createItem(itemDraft()) },
            Call(HttpMethod.Put, items) { it.catalog.replaceItem(ITEM_ID, VERSION, itemDraft()) },
            Call(HttpMethod.Put, "$items/published") { it.catalog.setItemPublished(ITEM_ID, true, VERSION) },
            Call(HttpMethod.Post, "$items/archive") { it.catalog.archiveItem(ITEM_ID, VERSION) },
            Call(HttpMethod.Post, "$items/restore") { it.catalog.restoreItem(ITEM_ID, VERSION) },
            Call(HttpMethod.Get, "$CATALOG/kits") { it.catalog.kitsPage(KitFilter(), null, LIMIT) },
            Call(HttpMethod.Get, kits) { it.catalog.kit(KIT_ID) },
            Call(HttpMethod.Post, "$CATALOG/kits") { it.catalog.createKit(kitDraft()) },
            Call(HttpMethod.Put, kits) { it.catalog.replaceKit(KIT_ID, VERSION, kitDraft()) },
            Call(HttpMethod.Put, "$kits/published") { it.catalog.setKitPublished(KIT_ID, true, VERSION) },
            Call(HttpMethod.Post, "$kits/archive") { it.catalog.archiveKit(KIT_ID, VERSION) },
            Call(HttpMethod.Post, "$kits/restore") { it.catalog.restoreKit(KIT_ID, VERSION) },
            Call(HttpMethod.Put, "$items/photos/order") { it.catalog.orderPhotos(item, listOf(PHOTO_ID)) },
            Call(HttpMethod.Put, "$kits/photos/order") { it.catalog.orderPhotos(kit, listOf(PHOTO_ID)) },
            Call(HttpMethod.Delete, "$items/photos/$PHOTO_ID") { it.catalog.deletePhoto(item, PHOTO_ID) },
            Call(HttpMethod.Delete, "$kits/photos/$PHOTO_ID") { it.catalog.deletePhoto(kit, PHOTO_ID) },
            Call(HttpMethod.Get, SUPPLIERS) { it.suppliers.page(null, null, LIMIT) },
            Call(HttpMethod.Get, "$SUPPLIERS/$SUPPLIER_ID") { it.suppliers.supplier(SUPPLIER_ID) },
            Call(HttpMethod.Post, SUPPLIERS) { it.suppliers.create(supplier) },
            Call(HttpMethod.Put, "$SUPPLIERS/$SUPPLIER_ID") { it.suppliers.replace(SUPPLIER_ID, 2, supplier) },
        )

    /** The answer a route gives: a list for a list read, the row for anything on one row, nothing for a delete. */
    private fun MockRequestHandleScope.answer(request: HttpRequestData): HttpResponseData {
        val path = request.url.encodedPath
        val created = if (request.method == HttpMethod.Post) HttpStatusCode.Created else HttpStatusCode.OK
        return when {
            request.method == HttpMethod.Delete -> respond("", HttpStatusCode.NoContent)
            path.endsWith("/photos/order") -> respondJson("""{"items": [$PHOTO_JSON]}""")
            request.method == HttpMethod.Get && path.substringAfterLast('/') in LISTS ->
                respondJson("""{"items": []}""")
            "/categories" in path -> respondJson(CATEGORY_JSON, created)
            "/attributes" in path -> respondJson(ATTRIBUTE_JSON, created)
            "/kits" in path -> respondJson(kitJson(), created)
            "/suppliers" in path -> respondJson(SUPPLIER_JSON, created)
            else -> respondJson(itemJson(), created)
        }
    }

    @Test
    fun everyOperationReachesItsRoute() =
        runTest {
            for (call in calls) {
                val wired = catalogWiring()
                wired.api = { request -> answer(request) }

                call.run(wired)

                val request = wired.apiRequests.single()
                assertEquals(call.method, request.method, "${call.method} ${call.path}")
                assertEquals(call.path, request.url.encodedPath, "${call.method} ${call.path}")
            }
        }

    @Test
    fun everyRefusalIsAFailureNotAnApiError() =
        runTest {
            for (call in calls) {
                val wired = catalogWiring()
                wired.api = { respondProblem(500, "internal") }

                val failure = assertFailsWith<Failure>("${call.method} ${call.path}") { call.run(wired) }

                assertTrue(failure is Failure.Unexpected, "${call.method} ${call.path}: $failure")
                assertFalse(wired.apiRequests.isEmpty())
            }
        }

    private companion object {
        const val CATALOG = "/api/v1/admin/catalog"
        const val SUPPLIERS = "/api/v1/admin/suppliers"
        const val POSITION = 2
        const val LIMIT = 20
        const val VERSION = 3
        val LISTS = setOf("categories", "attributes", "items", "kits", "suppliers")
    }
}
