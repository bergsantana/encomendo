package br.com.encomendo.core.network

import io.ktor.client.HttpClient
import io.ktor.client.call.body
import io.ktor.client.engine.mock.MockEngine
import io.ktor.client.engine.mock.MockEngineConfig
import io.ktor.client.engine.mock.MockRequestHandler
import io.ktor.client.request.HttpRequestData
import io.ktor.client.request.get
import io.ktor.client.statement.bodyAsText
import io.ktor.http.HttpHeaders
import io.ktor.http.HttpStatusCode
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.withContext
import kotlinx.coroutines.withTimeoutOrNull
import kotlinx.io.IOException
import kotlinx.serialization.Serializable
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertFalse
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.time.Duration.Companion.seconds

@OptIn(ExperimentalCoroutinesApi::class) // runCurrent
class BearerAuthTest {
    private val old = TestTokens(accessToken = "access-old", refreshToken = "refresh-old")
    private val fresh = TestTokens(accessToken = "access-new", refreshToken = "refresh-new")
    private val otherStore = TestTokens("access-other-store", "refresh-other-store", scope = "ana@moto-pecas")

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
    fun aRetryNeverCrossesIntoAnotherStore() =
        runTest {
            // A request made for one store, rejected while the session moved to another, must fail rather
            // than run in the new store with its token: the server could not tell (the web's scope check).
            val storage = InMemoryTokenStorage(old)
            val refresher = RecordingRefresher { error("the switch already rotated the token") }
            val seen = mutableListOf<String?>()
            val client =
                client(storage, refresher) { request ->
                    seen += request.bearer()
                    if (request.bearer() == old.accessToken) {
                        storage.overwrite(otherStore) // a switch landed while this request was on the wire
                        respondUnauthorized()
                    } else {
                        respondJson("{}")
                    }
                }

            val error = assertFailsWith<ApiError> { client.get("/api/v1/admin/orders") }

            assertEquals(401, error.status)
            assertEquals(listOf<String?>("access-old"), seen, "never retried with the other store's token")
            assertEquals(emptyList(), refresher.calls)
        }

    @Test
    fun theScreensOfTheStoredStoreSendAtOnce() =
        runTest {
            val turns = SessionTurns().apply { screensServe(old.scope) }
            val seen = mutableListOf<String?>()
            val client =
                pinnedClient(InMemoryTokenStorage(old), turns) { request ->
                    seen += request.bearer()
                    respondJson("{}")
                }

            client.get("/api/v1/admin/orders")

            assertEquals(listOf<String?>("access-old"), seen)
        }

    @Test
    fun aStoresTokenWaitsUntilTheScreensServeThatStore() =
        runTest {
            // After a switch, the new store's token waits until the root has replaced the old store's
            // screens (ADR 0018): until then, whoever asks may be one of those screens.
            val turns = SessionTurns().apply { screensServe(old.scope) }
            val seen = mutableListOf<String?>()
            val client =
                pinnedClient(InMemoryTokenStorage(otherStore), turns) { request ->
                    seen += request.bearer()
                    respondJson("{}")
                }

            val call = async { client.get("/api/v1/admin/orders") }
            runCurrent()
            assertEquals(emptyList(), seen, "nothing goes out while the old store's screens are on")

            turns.screensServe(otherStore.scope) // the root has replaced them
            call.await()

            assertEquals(listOf<String?>("access-other-store"), seen)
        }

    @Test
    fun aRequestOfScreensClearedDuringTheMoveIsNeverSent() =
        runTest {
            val turns = SessionTurns().apply { screensServe(old.scope) }
            val seen = mutableListOf<String?>()
            val client =
                pinnedClient(InMemoryTokenStorage(otherStore), turns) { request ->
                    seen += request.bearer()
                    respondJson("{}")
                }

            val call = launch { client.get("/api/v1/admin/orders") }
            runCurrent()
            call.cancel() // the root clears the old store's screens, then serves the new store
            turns.screensServe(otherStore.scope)
            call.join()
            runCurrent()

            assertEquals(emptyList(), seen)
        }

    @Test
    fun aRequestWaitingThroughASwitchAwayAndBackStillGoesOut() =
        runTest {
            // A to B and back before the root reported B: the screens never change, so the wait must look again.
            val turns = SessionTurns().apply { screensServe(old.scope) }
            val storage = InMemoryTokenStorage(otherStore)
            val seen = mutableListOf<String?>()
            val client =
                pinnedClient(storage, turns) { request ->
                    seen += request.bearer()
                    respondJson("{}")
                }

            val call = async { client.get("/api/v1/admin/orders") }
            runCurrent()
            assertEquals(emptyList(), seen)

            storage.overwrite(old) // back to the screens' own store
            advanceTimeBy(SessionTurns.RECHECK_MILLIS + 1)
            call.await()

            assertEquals(listOf<String?>("access-old"), seen)
        }

    @Test
    fun aReadOlderThanTheScreensStillGoesToItsOwnStore() =
        runTest {
            // The screens' scope is sampled before the read: a read that lands after the screens moved
            // still pairs with the scope it was read against, and never waits for a change already made.
            val turns = SessionTurns().apply { screensServe(old.scope) }
            val inner = InMemoryTokenStorage(old)
            val gate = CompletableDeferred<Unit>()
            val slowFirstRead =
                object : TokenStorage<TestTokens> {
                    private var first = true

                    override suspend fun read(): TestTokens? {
                        val value = inner.read()
                        if (first) {
                            first = false
                            gate.await()
                        }
                        return value
                    }

                    override suspend fun replace(
                        expected: TestTokens,
                        replacement: TestTokens?,
                    ): Boolean = inner.replace(expected, replacement)
                }
            val seen = mutableListOf<String?>()
            val client =
                pinnedClient(slowFirstRead, turns) { request ->
                    seen += request.bearer()
                    respondJson("{}")
                }

            val call = async { client.get("/api/v1/admin/orders") }
            runCurrent()
            inner.overwrite(otherStore) // a switch lands while the read is on its way back
            turns.screensServe(otherStore.scope)
            gate.complete(Unit)
            call.await()

            assertEquals(listOf<String?>("access-old"), seen, "sent at once, with the token read for A's screens")
            assertEquals(0L, testScheduler.currentTime, "no wait")
        }

    @Test
    fun aStoreSwitchedAwayAndBackNeverWaits() =
        runTest {
            // A to B and back before the root caught up: the stored token is the screens' own again.
            val turns = SessionTurns().apply { screensServe(old.scope) }
            val storage = InMemoryTokenStorage(otherStore)
            val client = pinnedClient(storage, turns) { respondJson("{}") }
            storage.overwrite(old)

            client.get("/api/v1/admin/orders")
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

    /** A client whose engine answers on the test's own scheduler, so "nothing went out" is certain. */
    private fun TestScope.pinnedClient(
        storage: TokenStorage<TestTokens>,
        turns: SessionTurns,
        handler: MockRequestHandler,
    ): HttpClient =
        createApiClient(
            engine =
                MockEngine(
                    MockEngineConfig().apply {
                        dispatcher = StandardTestDispatcher(testScheduler)
                        addHandler(handler)
                    },
                ),
            config = ApiClientConfig(baseUrl = API_URL),
            tokens = storage,
            refresher = RecordingRefresher { error("no refresh expected") },
            turns = turns,
        )

    private fun client(
        storage: TokenStorage<TestTokens>,
        refresher: TokenRefresher<TestTokens> = RecordingRefresher { error("no refresh expected") },
        turns: SessionTurns = SessionTurns(),
        handler: MockRequestHandler,
    ): HttpClient =
        createApiClient(
            engine = MockEngine(handler),
            config = ApiClientConfig(baseUrl = API_URL),
            tokens = storage,
            refresher = refresher,
            turns = turns,
        )
}

@OptIn(ExperimentalCoroutinesApi::class) // runCurrent
class BearerRefreshTest {
    private val old = TestTokens(accessToken = "access-old", refreshToken = "refresh-old")
    private val fresh = TestTokens(accessToken = "access-new", refreshToken = "refresh-new")
    private val client = HttpClient(MockEngine { error("the refresher is faked") })

    @Test
    fun reusesTokensAnotherRequestAlreadyRefreshed() =
        runTest {
            val refresher = RecordingRefresher { error("already refreshed") }
            val refresh = BearerRefresh(InMemoryTokenStorage(fresh), refresher, SessionTurns())
            refresh.issued(old)

            assertEquals(fresh, refresh.refresh(client, rejectedAccessToken = old.accessToken))
            assertEquals(emptyList(), refresher.calls)
        }

    @Test
    fun neverHandsARetryTheTokensOfAnotherStore() =
        runTest {
            val otherStore = TestTokens("access-other-store", "refresh-other-store", scope = "ana@moto-pecas")
            val refresher = RecordingRefresher { error("the switch already rotated the token") }
            val refresh = BearerRefresh(InMemoryTokenStorage(otherStore), refresher, SessionTurns())
            refresh.issued(old)

            assertNull(refresh.refresh(client, rejectedAccessToken = old.accessToken))
            assertEquals(emptyList(), refresher.calls)
        }

    @Test
    fun aRejectedTokenItNeverHandedOutIsNeverRetried() =
        runTest {
            // Its store is unknown, so it can't be shown to match.
            val refresh =
                BearerRefresh(InMemoryTokenStorage(fresh), RecordingRefresher { error("no refresh") }, SessionTurns())

            assertNull(refresh.refresh(client, rejectedAccessToken = "access-from-nowhere"))
        }

    @Test
    fun aRefreshThatLeavesTheStoreKeepsItsTokensButNeverRetries() =
        runTest {
            // Removed from the store: the refresh answers with no store (ADR 0010). The rotated tokens must be
            // kept, but the request was meant for the store it no longer has.
            val storage = InMemoryTokenStorage(old)
            val noStore = TestTokens("access-no-store", "refresh-no-store", scope = "ana@")
            val refresh = BearerRefresh(storage, RecordingRefresher { noStore }, SessionTurns())
            refresh.issued(old)

            assertNull(refresh.refresh(client, rejectedAccessToken = old.accessToken))
            assertEquals(noStore, storage.current)
        }

    @Test
    fun treatsBadRequestAndForbiddenAsARejectedRefreshToken() =
        runTest {
            for (status in listOf(HttpStatusCode.BadRequest, HttpStatusCode.Unauthorized, HttpStatusCode.Forbidden)) {
                val storage = InMemoryTokenStorage(old)
                val refresher =
                    RecordingRefresher { throw ApiError(status = status.value, code = ApiErrorCode.Unknown) }

                assertNull(
                    BearerRefresh(
                        storage,
                        refresher,
                        SessionTurns(),
                    ).refresh(client, rejectedAccessToken = old.accessToken),
                )
                assertNull(storage.current, "status ${status.value} must end the session")
            }
        }

    @Test
    fun aRefreshWaitsForAStoreSwitchAndNeverSpendsTheTokenItRotated() =
        runTest {
            // A switch spends the refresh token too (ADR 0010). If a refresh presented the same token
            // meanwhile, one of the two would hold a superseded token and sign the user out (ADR 0013).
            // And the request, meant for the old store, must not be retried in the new one.
            val turns = SessionTurns()
            val storage = InMemoryTokenStorage(old)
            val refresher = RecordingRefresher { error("the switch already rotated the token") }
            val refresh = BearerRefresh(storage, refresher, turns).apply { issued(old) }
            val switched = TestTokens("access-switched", "refresh-switched", scope = "ana@moto-pecas")
            val switchDone = CompletableDeferred<Unit>()
            launch {
                turns.take {
                    switchDone.await()
                    storage.overwrite(switched)
                }
            }
            runCurrent()

            val waiting = async { refresh.refresh(client, rejectedAccessToken = old.accessToken) }
            runCurrent()
            assertFalse(waiting.isCompleted, "the refresh waits its turn")

            switchDone.complete(Unit)

            assertNull(waiting.await(), "no retry in the store it moved to")
            assertEquals(emptyList(), refresher.calls)
            assertEquals(switched, storage.current)
        }

    @Test
    fun keepsTheSessionWhenRateLimited() =
        runTest {
            val storage = InMemoryTokenStorage(old)
            val refresher = RecordingRefresher { throw ApiError(status = 429, code = ApiErrorCode.Unknown) }

            val error =
                assertFailsWith<ApiError> {
                    BearerRefresh(
                        storage,
                        refresher,
                        SessionTurns(),
                    ).refresh(client, rejectedAccessToken = old.accessToken)
                }

            assertEquals(429, error.status)
            assertEquals(old, storage.current)
        }
}
