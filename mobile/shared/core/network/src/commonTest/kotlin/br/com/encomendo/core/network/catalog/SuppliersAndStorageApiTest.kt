package br.com.encomendo.core.network.catalog

import br.com.encomendo.core.network.ApiError
import br.com.encomendo.core.network.ApiErrorCode
import br.com.encomendo.core.network.respondJson
import io.ktor.client.HttpClient
import io.ktor.client.engine.mock.MockEngine
import io.ktor.client.engine.mock.respond
import io.ktor.client.request.HttpRequestData
import io.ktor.http.HttpHeaders
import io.ktor.http.HttpMethod
import io.ktor.http.HttpStatusCode
import io.ktor.http.content.OutgoingContent
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertContentEquals
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNull

class SuppliersAndStorageApiTest {
    private val requests = mutableListOf<HttpRequestData>()

    private fun suppliers(body: String = SUPPLIER_JSON) =
        SuppliersApi(
            catalogClient { request ->
                requests += request
                respondJson(body)
            },
        )

    @Test
    fun listsSuppliersByName() =
        runTest {
            val page = suppliers("""{"items": [$SUPPLIER_JSON], "nextCursor": "c2"}""").list("couro", "c1", 20)

            val request = requests.single()
            assertEquals("/api/v1/admin/suppliers", request.url.encodedPath)
            assertEquals("couro", request.url.parameters["q"])
            assertEquals("c1", request.url.parameters["cursor"])
            assertEquals("5511987654321", page.items.single().whatsappPhone)
            assertEquals("c2", page.nextCursor)
        }

    @Test
    fun readsCreatesAndReplacesASupplier() =
        runTest {
            suppliers().get(SUPPLIER_ID)
            assertEquals("/api/v1/admin/suppliers/$SUPPLIER_ID", requests.single().url.encodedPath)

            requests.clear()
            suppliers().create(SupplierWriteDto("Couros Paulista", "5511987654321"))
            assertEquals(HttpMethod.Post, requests.single().method)
            assertEquals("""{"name":"Couros Paulista","whatsappPhone":"5511987654321"}""", requests.single().bodyText())

            requests.clear()
            suppliers().replace(SUPPLIER_ID, SupplierWriteDto("Couros", "5511987654321", version = 2))
            assertEquals(HttpMethod.Put, requests.single().method)
            assertEquals(
                """{"name":"Couros","whatsappPhone":"5511987654321","version":2}""",
                requests.single().bodyText(),
            )
        }

    @Test
    fun theUploadCarriesTheReturnedHeadersAndNoBearer() =
        runTest {
            val sent = mutableListOf<HttpRequestData>()
            val storage = storage(sent, HttpStatusCode.OK)

            storage.put(
                uploadUrl = "https://files.test/bucket/key?X-Sig=abc",
                headers = mapOf("Content-Type" to "image/png", "x-amz-meta-owner" to "item"),
                bytes = byteArrayOf(1, 2, 3),
            )

            val request = sent.single()
            assertEquals(HttpMethod.Put, request.method)
            assertEquals("files.test", request.url.host)
            assertEquals("abc", request.url.parameters["X-Sig"])
            assertNull(request.headers[HttpHeaders.Authorization], "the storage host never sees the bearer token")
            assertEquals("image/png", request.body.contentType?.toString())
            assertEquals("item", request.headers["x-amz-meta-owner"])
            assertContentEquals(byteArrayOf(1, 2, 3), (request.body as OutgoingContent.ByteArrayContent).bytes())
        }

    @Test
    fun aRefusedUploadIsAnUploadFailure() =
        runTest {
            val storage = storage(mutableListOf(), HttpStatusCode.Forbidden)

            val error = assertFailsWith<ApiError> { storage.put("https://files.test/k", emptyMap(), byteArrayOf(1)) }

            assertEquals(ApiErrorCode.UploadFailed, error.code)
            assertEquals(403, error.status)
        }

    @Test
    fun anUploadThatNeverGotThereIsANetworkError() =
        runTest {
            val storage =
                PhotoStorageApi(HttpClient(MockEngine { throw kotlinx.io.IOException("no route") }))

            val error = assertFailsWith<ApiError> { storage.put("https://files.test/k", emptyMap(), byteArrayOf(1)) }

            assertEquals(ApiErrorCode.Network, error.code)
        }

    private fun storage(
        sent: MutableList<HttpRequestData>,
        status: HttpStatusCode,
    ) = PhotoStorageApi(
        createStorageClient(
            MockEngine { request ->
                sent += request
                respond("", status)
            },
        ),
    )
}
