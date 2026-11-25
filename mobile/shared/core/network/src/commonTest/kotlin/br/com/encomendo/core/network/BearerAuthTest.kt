package br.com.encomendo.core.network

import io.ktor.client.HttpClient
import io.ktor.client.call.body
import io.ktor.client.engine.mock.MockEngine
import io.ktor.client.engine.mock.MockRequestHandler
import io.ktor.client.request.HttpRequestData
import io.ktor.client.request.get
import io.ktor.client.statement.bodyAsText
import io.ktor.http.HttpHeaders
import io.ktor.http.HttpStatusCode
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.withContext
import kotlinx.coroutines.withTimeoutOrNull
import kotlinx.io.IOException
import kotlinx.serialization.Serializable
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.time.Duration.Companion.seconds

class BearerAuthTest {
    private val old = TestTokens(accessToken = "access-old", refreshToken = "refresh-old")
    private val fresh = TestTokens(accessToken = "access-new", refreshToken = "refresh-new")

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
            val refresher = RecordingRefresher { throw ApiError(status = 401, code = ApiErrorCode("unauthenticated")) }
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

            // The caller sees why it failed (offline), not a 401 that would read as "session expired".
            assertEquals(ApiErrorCode.Network, error.code)
            assertEquals(old, storage.current, "a workshop with weak signal must not be logged out")
        }

    @Test
    fun keepsTheSessionWhenTheServerFailsDuringRefresh() =
        runTest {
            val storage = InMemoryTokenStorage(old)
            val refresher = RecordingRefresher { throw ApiError(status = 503, code = ApiErrorCode.NotReady) }
            val client = client(storage, refresher) { respondUnauthorized() }

            val error = assertFailsWith<ApiError> { client.get("/api/v1/admin/orders") }

            assertEquals(503, error.status)
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
    fun aLogoutDuringARefreshStaysALogout() =
        runTest {
            val storage = InMemoryTokenStorage(old)
            val started = CompletableDeferred<Unit>()
            val gate = CompletableDeferred<Unit>()
            val refresher =
                RecordingRefresher {
                    started.complete(Unit)
                    gate.await()
                    fresh
                }
            val client = client(storage, refresher) { respondUnauthorized() }

            val call = async { assertFailsWith<ApiError> { client.get("/api/v1/admin/orders") } }
            started.await()
            storage.overwrite(null) // "Sair" while the refresh is on the wire
            gate.complete(Unit)

            assertEquals(401, call.await().status)
            assertNull(storage.current, "the refreshed pair must not sign the user back in")
        }

    @Test
    fun aLoginDuringARejectedRefreshIsKept() =
        runTest {
            val other = TestTokens(accessToken = "access-other", refreshToken = "refresh-other")
            val storage = InMemoryTokenStorage(old)
            val started = CompletableDeferred<Unit>()
            val gate = CompletableDeferred<Unit>()
            val refresher =
                RecordingRefresher {
                    started.complete(Unit)
                    gate.await()
                    throw ApiError(status = 401, code = ApiErrorCode("unauthenticated"))
                }
            val client = client(storage, refresher) { respondUnauthorized() }

            val call = async { assertFailsWith<ApiError> { client.get("/api/v1/admin/orders") } }
            started.await()
            storage.overwrite(other) // a new login while the old session's refresh is refused
            gate.complete(Unit)
            call.await()

            assertEquals(other, storage.current, "a refused old refresh must not end the new session")
        }

    @Test
    fun aRefusedRefreshStillFailsAsRefusedWhenSigningOutCannotBeSaved() =
        runTest {
            // The server said the session is over; a full disk must not turn that into "offline".
            val storage =
                object : TokenStorage<TestTokens> by InMemoryTokenStorage(old) {
                    override suspend fun replace(
                        expected: TestTokens,
                        replacement: TestTokens?,
                    ): Boolean = throw IOException("disk full")
                }
            val refresher = RecordingRefresher { throw ApiError(status = 401, code = ApiErrorCode("unauthenticated")) }
            val client = client(storage, refresher) { respondUnauthorized() }

            val error = assertFailsWith<ApiError> { client.get("/api/v1/admin/orders") }

            assertEquals(401, error.status)
        }

    @Test
    fun leavingAScreenDuringARefreshStillKeepsTheRotatedTokens() =
        runTest {
            // The server has already spent refresh-old; dropping its answer would leave a
            // spent token behind, and reusing it revokes the whole family.
            val storage = InMemoryTokenStorage(old)
            val started = CompletableDeferred<Unit>()
            val gate = CompletableDeferred<Unit>()
            val refresher =
                RecordingRefresher {
                    started.complete(Unit)
                    gate.await()
                    fresh
                }
            val client = client(storage, refresher) { respondUnauthorized() }

            val call = launch { client.get("/api/v1/admin/orders") }
            started.await()
            call.cancel()
            gate.complete(Unit)
            call.join()

            assertEquals(fresh, storage.current)
            assertEquals(listOf("refresh-old"), refresher.calls)
        }

    @Test
    fun aRefresherThatForgetsWithoutBearerFailsInsteadOfHanging() =
        runTest {
            val storage = InMemoryTokenStorage(old)
            val careless =
                TokenRefresher<TestTokens> {
                    client,
                    _,
                    ->
                    client.get("/auth/refresh").body<AuthTokensBody>().toTokens()
                }
            val client = client(storage, careless) { respondUnauthorized() }

            // A deadlocked refresh is non-cancellable, so it runs in a detached scope and the
            // test waits in real time: a regression fails here instead of hanging the build.
            val call =
                CoroutineScope(Dispatchers.Default).async {
                    try {
                        client.get("/api/v1/admin/orders")
                        null
                    } catch (error: ApiError) {
                        error
                    }
                }
            val error = withContext(Dispatchers.Default) { withTimeoutOrNull(5.seconds) { call.await() } }

            assertEquals(401, assertNotNull(error, "the refresh deadlocked").status)
            assertNull(storage.current)
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

    @Serializable
    private data class AuthTokensBody(
        val accessToken: String,
        val refreshToken: String,
    ) {
        fun toTokens() = TestTokens(accessToken = accessToken, refreshToken = refreshToken)
    }

    private fun HttpRequestData.bearer(): String? = headers[HttpHeaders.Authorization]?.removePrefix("Bearer ")

    private fun client(
        storage: TokenStorage<TestTokens>,
        refresher: TokenRefresher<TestTokens> = RecordingRefresher { error("no refresh expected") },
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
    private val old = TestTokens(accessToken = "access-old", refreshToken = "refresh-old")
    private val fresh = TestTokens(accessToken = "access-new", refreshToken = "refresh-new")
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

            val error =
                assertFailsWith<ApiError> {
                    BearerRefresh(storage, refresher).refresh(client, rejectedAccessToken = old.accessToken)
                }

            assertEquals(429, error.status)
            assertEquals(old, storage.current)
        }
}
