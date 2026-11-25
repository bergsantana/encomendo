package br.com.encomendo.core.network

import io.ktor.client.HttpClient
import io.ktor.client.engine.mock.MockEngine
import io.ktor.client.engine.mock.MockRequestHandler
import io.ktor.client.request.HttpRequestData
import io.ktor.client.request.get
import io.ktor.client.statement.bodyAsText
import io.ktor.http.HttpHeaders
import io.ktor.http.HttpStatusCode
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNull

class BearerAuthTest {
    private val old = AuthTokens(accessToken = "access-old", refreshToken = "refresh-old")
    private val fresh = AuthTokens(accessToken = "access-new", refreshToken = "refresh-new")

    @Test
    fun sendsTheStoredAccessToken() =
        runTest {
            val seen = mutableListOf<String?>()
            val client =
                client(InMemoryTokenStorage(old)) { request ->
                    seen += request.bearer()
                    respondJson("{}")
                }

            client.get("/api/v1/admin/orders")

            assertEquals(listOf<String?>("access-old"), seen)
        }

    @Test
    fun sendsNoTokenWhenSignedOut() =
        runTest {
            val refresher = RecordingRefresher { error("no refresh without a session") }
            val seen = mutableListOf<String?>()
            val client =
                client(InMemoryTokenStorage(), refresher) { request ->
                    seen += request.bearer()
                    respondUnauthorized()
                }

            val error = assertFailsWith<ApiError> { client.get("/api/v1/admin/orders") }

            assertEquals(401, error.status)
            assertEquals(listOf<String?>(null), seen)
            assertEquals(emptyList(), refresher.calls)
        }

    @Test
    fun neverSendsTokensToAnotherOrigin() =
        runTest {
            val refresher = RecordingRefresher { error("a foreign 401 must not refresh") }
            val seen = mutableListOf<String?>()
            val client =
                client(InMemoryTokenStorage(old), refresher) { request ->
                    seen += request.bearer()
                    respondUnauthorized()
                }

            assertFailsWith<ApiError> { client.get("https://photos.example/item.jpg") }
            assertFailsWith<ApiError> { client.get("http://api.test/api/v1/admin/orders") } // same host, no TLS

            assertEquals(listOf<String?>(null, null), seen)
            assertEquals(emptyList(), refresher.calls)
        }

    @Test
    fun refreshesOnceOn401AndRetriesWithTheNewToken() =
        runTest {
            val storage = InMemoryTokenStorage(old)
            val refresher = RecordingRefresher { fresh }
            val seen = mutableListOf<String?>()
            val client =
                client(storage, refresher) { request ->
                    seen += request.bearer()
                    if (request.bearer() == fresh.accessToken) respondJson("""{"ok":true}""") else respondUnauthorized()
                }

            val body = client.get("/api/v1/admin/orders").bodyAsText()

            assertEquals("""{"ok":true}""", body)
            assertEquals(listOf<String?>("access-old", "access-new"), seen)
            assertEquals(listOf("refresh-old"), refresher.calls)
            assertEquals(fresh, storage.current)
        }

    @Test
    fun logsOutWhenTheServerRejectsTheRefreshToken() =
        runTest {
            val storage = InMemoryTokenStorage(old)
            val refresher = RecordingRefresher { throw ApiError(status = 401, code = ApiErrorCode("unauthorized")) }
            val client = client(storage, refresher) { respondUnauthorized() }

            val error = assertFailsWith<ApiError> { client.get("/api/v1/admin/orders") }

            assertEquals(401, error.status)
            assertNull(storage.current, "a rejected refresh must end the session")
        }

    @Test
    fun keepsTheSessionWhenTheRefreshCannotReachTheServer() =
        runTest {
            val storage = InMemoryTokenStorage(old)
            val refresher = RecordingRefresher { throw ApiError(status = 0, code = ApiErrorCode.Network) }
            val client = client(storage, refresher) { respondUnauthorized() }

            val error = assertFailsWith<ApiError> { client.get("/api/v1/admin/orders") }

            assertEquals(401, error.status)
            assertEquals(old, storage.current, "a workshop with weak signal must not be logged out")
        }

    @Test
    fun keepsTheSessionWhenTheServerFailsDuringRefresh() =
        runTest {
            val storage = InMemoryTokenStorage(old)
            val refresher = RecordingRefresher { throw ApiError(status = 503, code = ApiErrorCode.NotReady) }
            val client = client(storage, refresher) { respondUnauthorized() }

            assertFailsWith<ApiError> { client.get("/api/v1/admin/orders") }

            assertEquals(old, storage.current)
        }

    @Test
    fun refreshesOnlyOnceWhenTheRetryIsRejectedToo() =
        runTest {
            val refresher = RecordingRefresher { fresh }
            val client = client(InMemoryTokenStorage(old), refresher) { respondUnauthorized() }

            val error = assertFailsWith<ApiError> { client.get("/api/v1/admin/orders") }

            assertEquals(401, error.status)
            assertEquals(listOf("refresh-old"), refresher.calls)
        }

    @Test
    fun concurrentUnauthorizedRequestsShareOneRefresh() =
        runTest {
            // Refresh tokens rotate and a reused one revokes the whole family
            // (docs/architecture.md 5.1), so two refreshes with the same token log the user out.
            val refresher = RecordingRefresher { fresh }
            val rejected = MutableStateFlow(0)
            val client =
                client(InMemoryTokenStorage(old), refresher) { request ->
                    if (request.bearer() == fresh.accessToken) return@client respondJson("{}")
                    rejected.update { it + 1 }
                    rejected.first { it >= 2 } // both requests are in flight with the old token
                    respondUnauthorized()
                }

            listOf(
                async { client.get("/api/v1/admin/orders") },
                async { client.get("/api/v1/admin/customers") },
            ).awaitAll()

            assertEquals(listOf("refresh-old"), refresher.calls)
        }

    @Test
    fun requestsMarkedWithoutBearerCarryNoTokenAndNeverRefresh() =
        runTest {
            val refresher = RecordingRefresher { error("a failed login must not refresh") }
            val seen = mutableListOf<String?>()
            val client =
                client(InMemoryTokenStorage(old), refresher) { request ->
                    seen += request.bearer()
                    respondUnauthorized()
                }

            assertFailsWith<ApiError> { client.get("/auth/login") { withoutBearer() } }

            assertEquals(listOf<String?>(null), seen)
            assertEquals(emptyList(), refresher.calls)
        }

    private fun HttpRequestData.bearer(): String? = headers[HttpHeaders.Authorization]?.removePrefix("Bearer ")

    private fun client(
        storage: TokenStorage,
        refresher: TokenRefresher = RecordingRefresher { error("no refresh expected") },
        handler: MockRequestHandler,
    ): HttpClient =
        createApiClient(
            engine = MockEngine(handler),
            config = ApiClientConfig(baseUrl = API_URL),
            tokens = storage,
            refresher = refresher,
        )
}

class BearerRefreshTest {
    private val old = AuthTokens(accessToken = "access-old", refreshToken = "refresh-old")
    private val fresh = AuthTokens(accessToken = "access-new", refreshToken = "refresh-new")
    private val client = HttpClient(MockEngine { error("the refresher is faked") })

    @Test
    fun reusesTokensAnotherRequestAlreadyRefreshed() =
        runTest {
            val refresher = RecordingRefresher { error("already refreshed") }
            val refresh = BearerRefresh(InMemoryTokenStorage(fresh), refresher)

            assertEquals(fresh, refresh.refresh(client, rejectedAccessToken = old.accessToken))
            assertEquals(emptyList(), refresher.calls)
        }

    @Test
    fun treatsBadRequestAndForbiddenAsARejectedRefreshToken() =
        runTest {
            for (status in listOf(HttpStatusCode.BadRequest, HttpStatusCode.Unauthorized, HttpStatusCode.Forbidden)) {
                val storage = InMemoryTokenStorage(old)
                val refresher =
                    RecordingRefresher { throw ApiError(status = status.value, code = ApiErrorCode.Unknown) }

                assertNull(BearerRefresh(storage, refresher).refresh(client, rejectedAccessToken = old.accessToken))
                assertNull(storage.current, "status ${status.value} must end the session")
            }
        }

    @Test
    fun keepsTheSessionWhenRateLimited() =
        runTest {
            val storage = InMemoryTokenStorage(old)
            val refresher = RecordingRefresher { throw ApiError(status = 429, code = ApiErrorCode.Unknown) }

            assertNull(BearerRefresh(storage, refresher).refresh(client, rejectedAccessToken = old.accessToken))
            assertEquals(old, storage.current)
        }
}
