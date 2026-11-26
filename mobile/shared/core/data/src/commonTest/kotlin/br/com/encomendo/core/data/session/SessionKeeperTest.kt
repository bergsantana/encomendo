package br.com.encomendo.core.data.session

import br.com.encomendo.core.domain.session.GrantedPermissions
import br.com.encomendo.core.domain.session.Permission
import br.com.encomendo.core.network.ApiClientConfig
import br.com.encomendo.core.network.SessionTurns
import br.com.encomendo.core.network.auth.AuthApi
import br.com.encomendo.core.security.SessionTokens
import br.com.encomendo.core.security.StoredSession
import br.com.encomendo.core.security.TokenStore
import br.com.encomendo.core.testing.AltaRpm
import br.com.encomendo.core.testing.FakeSecretCipher
import br.com.encomendo.core.testing.temporaryPreferencesDataStore
import br.com.encomendo.core.testing.testSession
import io.ktor.client.engine.mock.MockEngine
import io.ktor.client.engine.mock.MockRequestHandleScope
import io.ktor.client.engine.mock.MockRequestHandler
import io.ktor.client.engine.mock.respond
import io.ktor.client.engine.mock.toByteArray
import io.ktor.client.request.HttpResponseData
import io.ktor.client.request.get
import io.ktor.http.HttpHeaders
import io.ktor.http.HttpStatusCode
import io.ktor.http.headersOf
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.withContext
import kotlinx.io.IOException
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull
import kotlin.test.assertTrue

@OptIn(ExperimentalCoroutinesApi::class) // testScheduler.currentTime
class SessionKeeperTest {
    private companion object {
        /** ADR 0013: presenting a used refresh token again within 2 minutes is a retry, not reuse. */
        const val RETRY_WINDOW_MILLIS = 120_000L

        /** Long enough for a renewal that skipped the turn to reach the server first. */
        const val SETTLE_MILLIS = 200L
    }

    private val signedIn =
        StoredSession(SessionTokens("access-old", "refresh-old"), testSession(permissions = setOf("order:read")))

    @Test
    fun aRenewalKeepsTheNewTokensAndWhatTheRoleAllowsNow() =
        runTest {
            // On app start: a role changed on the web reaches the phone without waiting for a 401 (ADR 0010).
            val store = tokenStore().apply { write(signedIn) }
            val bodies = mutableListOf<String>()
            val keeper =
                keeper(store) { request ->
                    bodies += request.body.toByteArray().decodeToString()
                    respondJson(renewedSession(permissions = """["order:read","order:write"]"""))
                }

            keeper.renew()

            assertEquals(listOf("""{"refreshToken":"refresh-old"}"""), bodies)
            assertEquals(SessionTokens("access-renewed", "refresh-renewed"), store.read()?.tokens)
            assertEquals(
                GrantedPermissions(listOf(Permission("order:read"), Permission("order:write"))),
                store.read()?.session?.permissions,
            )
        }

    @Test
    fun withNobodySignedInItAsksNothing() =
        runTest {
            val calls = mutableListOf<String>()
            val keeper =
                keeper(tokenStore()) { request ->
                    calls += request.url.encodedPath
                    respondJson("{}")
                }

            keeper.renew()

            assertEquals(emptyList(), calls)
        }

    @Test
    fun noSignalAServerFailureOrARateLimitKeepTheSession() =
        runTest {
            // The workshop loses signal; the app must open with what it had (ADR 0012).
            val answers =
                listOf<MockRequestHandler>(
                    { throw IOException("no signal") },
                    { respondProblem(HttpStatusCode.ServiceUnavailable, """{"status":503,"code":"not_ready"}""") },
                    { respondProblem(HttpStatusCode.TooManyRequests, """{"status":429,"code":"rate_limited"}""") },
                )
            for (answer in answers) {
                val store = tokenStore().apply { write(signedIn) }

                keeper(store, answer).renew()

                assertEquals(signedIn, store.read())
            }
        }

    @Test
    fun aLostAnswerIsTriedAgainWithinTheRetryWindow() =
        runTest {
            // The server may have rotated the token before the answer was lost. Presenting it again within
            // 2 minutes is a retry, not reuse (ADR 0013); waiting for the next 401, 15 minutes later, would
            // revoke the family and sign the user out.
            val store = tokenStore().apply { write(signedIn) }
            val calls = mutableListOf<Long>()
            val keeper =
                keeper(store) {
                    calls += testScheduler.currentTime
                    if (calls.size ==
                        1
                    ) {
                        throw IOException("the answer never came")
                    } else {
                        respondJson(renewedSession(permissions = "[]"))
                    }
                }

            keeper.renew()

            assertEquals(2, calls.size)
            assertTrue(calls.last() - calls.first() < RETRY_WINDOW_MILLIS, "tried again inside the retry window")
            assertEquals(SessionTokens("access-renewed", "refresh-renewed"), store.read()?.tokens)
        }

    @Test
    fun aServerThatKeepsFailingIsTriedAFewTimesThenLeftAlone() =
        runTest {
            val store = tokenStore().apply { write(signedIn) }
            val calls = mutableListOf<Long>()
            val keeper =
                keeper(store) {
                    calls += testScheduler.currentTime
                    respondProblem(HttpStatusCode.ServiceUnavailable, """{"status":503,"code":"not_ready"}""")
                }

            keeper.renew()

            assertEquals(3, calls.size)
            assertTrue(calls.last() - calls.first() < RETRY_WINDOW_MILLIS)
            assertEquals(signedIn, store.read())
        }

    @Test
    fun aRateLimitOrARefusalIsNeverTriedAgain() =
        runTest {
            // The server answered: a 429 asks to wait, a refusal is final (ADR 0012).
            for (status in listOf(HttpStatusCode.TooManyRequests, HttpStatusCode.Unauthorized)) {
                val calls = mutableListOf<String>()
                val keeper =
                    keeper(tokenStore().apply { write(signedIn) }) { request ->
                        calls += request.url.encodedPath
                        respondProblem(status, """{"status":${status.value},"code":"x"}""")
                    }

                keeper.renew()

                assertEquals(listOf("/auth/refresh"), calls, "status ${status.value}")
            }
        }

    @Test
    fun aRenewalWaitsForARefreshInFlightAndPresentsItsNewToken() =
        runTest {
            // Both spend the refresh token: presenting the old one twice would leave one of them with a
            // superseded token, and its next use would sign the user out (ADR 0013).
            val store = tokenStore().apply { write(signedIn) }
            val refreshing = CompletableDeferred<Unit>()
            val refreshMayAnswer = CompletableDeferred<Unit>()
            val refreshBodies = mutableListOf<String>()
            val turns = SessionTurns()
            val client =
                createSessionApiClient(
                    MockEngine { request ->
                        when {
                            request.url.encodedPath != "/auth/refresh" ->
                                if (request.headers[HttpHeaders.Authorization] == "Bearer access-old") {
                                    respondProblem(
                                        HttpStatusCode.Unauthorized,
                                        """{"status":401,"code":"unauthenticated"}""",
                                    )
                                } else {
                                    respondJson("{}")
                                }
                            refreshBodies.isEmpty() -> {
                                refreshBodies += request.body.toByteArray().decodeToString()
                                refreshing.complete(Unit)
                                refreshMayAnswer.await()
                                respondJson(renewedSession(permissions = "[]", token = "refreshed"))
                            }
                            else -> {
                                refreshBodies += request.body.toByteArray().decodeToString()
                                respondJson(renewedSession(permissions = "[]"))
                            }
                        }
                    },
                    ApiClientConfig(baseUrl = "https://api.test"),
                    store,
                    turns,
                )
            val keeper = SessionKeeper(auth = AuthApi(client), store = store, turns = turns)

            launch { client.get("/api/v1/admin/orders") }
            refreshing.await()
            val renewal = launch { keeper.renew() }
            // Real time, beyond the test scheduler: a renewal that skipped the turn would go out now.
            withContext(Dispatchers.Default) { delay(SETTLE_MILLIS) }
            refreshMayAnswer.complete(Unit)
            renewal.join()

            assertEquals(
                listOf("""{"refreshToken":"refresh-old"}""", """{"refreshToken":"refresh-refreshed"}"""),
                refreshBodies,
            )
        }

    @Test
    fun aRefusedRefreshTokenEndsTheSession() =
        runTest {
            for (status in listOf(HttpStatusCode.BadRequest, HttpStatusCode.Unauthorized, HttpStatusCode.Forbidden)) {
                val store = tokenStore().apply { write(signedIn) }

                keeper(
                    store,
                ) { respondProblem(status, """{"status":${status.value},"code":"unauthenticated"}""") }.renew()

                assertNull(store.read(), "status ${status.value} must end the session")
            }
        }

    @Test
    fun aRenewalTheCallerGaveUpOnStillKeepsTheRotatedToken() =
        runTest {
            // The server may rotate even if the app moves on; dropping the answer would leave a spent token.
            val store = tokenStore().apply { write(signedIn) }
            val reached = CompletableDeferred<Unit>()
            val answer = CompletableDeferred<Unit>()
            val keeper =
                keeper(store) {
                    reached.complete(Unit)
                    answer.await()
                    respondJson(renewedSession(permissions = "[]"))
                }

            val caller = launch { keeper.renew() }
            reached.await()
            caller.cancel()
            answer.complete(Unit)
            caller.join() // DataStore writes on a real IO thread

            assertEquals(SessionTokens("access-renewed", "refresh-renewed"), store.read()?.tokens)
        }

    @Test
    fun aLogoutDuringARenewalStaysALogout() =
        runTest {
            val store = tokenStore().apply { write(signedIn) }
            val reached = CompletableDeferred<Unit>()
            val answer = CompletableDeferred<Unit>()
            val keeper =
                keeper(store) {
                    reached.complete(Unit)
                    answer.await()
                    respondJson(renewedSession(permissions = "[]"))
                }

            val renewal = launch { keeper.renew() }
            reached.await()
            store.clear() // Sair, while the renewal is on the wire
            answer.complete(Unit)
            renewal.join()

            assertNull(store.read())
        }

    private fun TestScope.tokenStore(): TokenStore =
        TokenStore(
            dataStore = temporaryPreferencesDataStore(backgroundScope),
            cipher = FakeSecretCipher(),
            cryptoDispatcher = StandardTestDispatcher(testScheduler),
        )

    private fun keeper(
        store: TokenStore,
        handler: MockRequestHandler,
    ): SessionKeeper {
        val turns = SessionTurns()
        val client =
            createSessionApiClient(MockEngine(handler), ApiClientConfig(baseUrl = "https://api.test"), store, turns)
        return SessionKeeper(auth = AuthApi(client), store = store, turns = turns)
    }

    private fun MockRequestHandleScope.respondJson(body: String): HttpResponseData =
        respond(body, HttpStatusCode.OK, headersOf(HttpHeaders.ContentType, "application/json"))

    private fun MockRequestHandleScope.respondProblem(
        status: HttpStatusCode,
        body: String,
    ): HttpResponseData = respond(body, status, headersOf(HttpHeaders.ContentType, "application/problem+json"))

    private fun renewedSession(
        permissions: String,
        token: String = "renewed",
    ): String =
        """
        {"accessToken": "access-$token", "tokenType": "Bearer", "expiresIn": 900, "refreshToken": "refresh-$token",
         "user": {"id": "0190f3c4-0000-7000-8000-0000000000aa", "name": "Ana Souza", "email": "ana@altarpm.com.br"},
         "tenant": {"id": "${AltaRpm.id}", "name": "Alta RPM", "slug": "alta-rpm"},
         "role": "admin", "permissions": $permissions,
         "memberships": [{"tenant": {"id": "${AltaRpm.id}", "name": "Alta RPM", "slug": "alta-rpm"}, "role": "admin"}]}
        """
}
