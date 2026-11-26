package br.com.encomendo.core.data.session

import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.domain.session.Membership
import br.com.encomendo.core.domain.session.Role
import br.com.encomendo.core.domain.session.TenantSummary
import br.com.encomendo.core.domain.session.scope
import br.com.encomendo.core.network.ApiClientConfig
import br.com.encomendo.core.network.SessionTurns
import br.com.encomendo.core.network.auth.AuthApi
import br.com.encomendo.core.network.tenancy.TenancyApi
import br.com.encomendo.core.security.SessionTokens
import br.com.encomendo.core.security.StoredSession
import br.com.encomendo.core.security.TokenStore
import br.com.encomendo.core.testing.AltaRpm
import br.com.encomendo.core.testing.FakeSecretCipher
import br.com.encomendo.core.testing.temporaryPreferencesDataStore
import br.com.encomendo.core.testing.testSession
import io.ktor.client.HttpClient
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
import kotlinx.coroutines.async
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.withContext
import kotlinx.io.IOException
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

class ApiTenancyRepositoryTest {
    private val signedIn = StoredSession(SessionTokens("access-old", "refresh-old"), testSession(tenant = AltaRpm))

    @Test
    fun theNewStoresTokenWaitsUntilTheScreensServeIt() =
        runTest {
            // Until the root has replaced the old store's screens, none of them may send with it (ADR 0018).
            val store = tokenStore().apply { write(signedIn) }
            val turns = SessionTurns().apply { screensServe(signedIn.session.scope) }
            val sent = CompletableDeferred<String?>()
            val tenancy =
                repository(store, turns) { request ->
                    if (request.url.encodedPath == "/auth/switch-tenant") {
                        respondJson(switchedSession(refreshToken = "refresh-switched"))
                    } else {
                        sent.complete(request.headers[HttpHeaders.Authorization])
                        respondJson("{}")
                    }
                }
            tenancy.repository.switchTo(MOTO_PECAS.id)

            val call = async { tenancy.client.get("/api/v1/admin/orders") }
            // Real time: a request that skipped the check would go out now.
            withContext(Dispatchers.Default) { delay(SETTLE_MILLIS) }
            assertFalse(sent.isCompleted, "waits for the new store's screens")

            turns.screensServe(store.read()?.session?.scope) // what the root does once they are on
            call.await()

            assertEquals("Bearer access-switched", sent.await())
        }

    @Test
    fun aSwitchKeepsTheNewStoreWithItsTokens() =
        runTest {
            val store = tokenStore().apply { write(signedIn) }
            val bodies = mutableListOf<String>()
            val tenancy =
                repository(store) { request ->
                    bodies += request.body.toByteArray().decodeToString()
                    respondJson(switchedSession(refreshToken = "refresh-switched"))
                }

            val session = tenancy.switchTo(MOTO_PECAS.id)

            assertEquals(listOf("""{"tenantId":"${MOTO_PECAS.id}","refreshToken":"refresh-old"}"""), bodies)
            assertEquals(MOTO_PECAS, session.tenant)
            assertEquals(SessionTokens("access-switched", "refresh-switched"), store.read()?.tokens)
            assertEquals(MOTO_PECAS, store.read()?.session?.tenant)
        }

    @Test
    fun aRefusedSwitchKeepsTheStoreAndRenewsTheListOfStores() =
        runTest {
            // ADR 0013: a 403 from switch-tenant leaves the refresh token and the store as they were. The
            // store was probably lost, so the session is renewed once and the list drops it, as on the web.
            val bothStores =
                signedIn.copy(
                    session =
                        signedIn.session.copy(
                            memberships = signedIn.session.memberships + Membership(MOTO_PECAS, Role("staff")),
                        ),
                )
            val store = tokenStore().apply { write(bothStores) }
            val calls = mutableListOf<String>()
            val tenancy =
                repository(store) { request ->
                    calls += request.url.encodedPath
                    if (request.url.encodedPath == "/auth/switch-tenant") {
                        respondProblem(
                            HttpStatusCode.Forbidden,
                            """{"title":"Sem acesso","status":403,"code":"forbidden"}""",
                        )
                    } else {
                        respondJson(refreshedSession())
                    }
                }

            assertFailsWith<Failure.Forbidden> { tenancy.switchTo(MOTO_PECAS.id) }

            assertEquals(listOf("/auth/switch-tenant", "/auth/refresh"), calls)
            assertEquals(SessionTokens("access-refreshed", "refresh-refreshed"), store.read()?.tokens)
            assertEquals(AltaRpm, store.read()?.session?.tenant)
            assertEquals(
                listOf(AltaRpm),
                store
                    .read()
                    ?.session
                    ?.memberships
                    ?.map { it.tenant },
            )
        }

    @Test
    fun anInvalidSwitchRenewsNothing() =
        runTest {
            val store = tokenStore().apply { write(signedIn) }
            val calls = mutableListOf<String>()
            val tenancy =
                repository(store) { request ->
                    calls += request.url.encodedPath
                    respondProblem(
                        HttpStatusCode.BadRequest,
                        """{"title":"Dados inválidos","status":400,"code":"invalid"}""",
                    )
                }

            assertFailsWith<Failure.InvalidInput> { tenancy.switchTo("not-a-store") }

            assertEquals(listOf("/auth/switch-tenant"), calls)
            assertEquals(signedIn, store.read())
        }

    @Test
    fun anOfflineSwitchKeepsTheSession() =
        runTest {
            val store = tokenStore().apply { write(signedIn) }
            val tenancy = repository(store) { throw IOException("no signal") }

            assertFailsWith<Failure.Offline> { tenancy.switchTo(MOTO_PECAS.id) }

            assertEquals(signedIn, store.read())
        }

    @Test
    fun aRefreshTokenTheServerRefusesEndsTheSession() =
        runTest {
            // A 401 means the refresh token itself was refused, as on /auth/refresh: it can't be used again.
            val store = tokenStore().apply { write(signedIn) }
            val tenancy =
                repository(store) {
                    respondProblem(
                        HttpStatusCode.Unauthorized,
                        """{"title":"Sessão expirada","status":401,"code":"unauthenticated"}""",
                    )
                }

            val failure = assertFailsWith<Failure.Unexpected> { tenancy.switchTo(MOTO_PECAS.id) }

            assertEquals(401, failure.status)
            assertNull(store.read())
        }

    @Test
    fun aSwitchTheScreenGaveUpOnStillKeepsTheRotatedToken() =
        runTest {
            // Back during "Abrindo…": the server may already have rotated the token. Dropping its answer
            // would leave the device with a spent token, and the next refresh, minutes later and past the
            // retry window, would revoke the family (ADR 0013).
            val store = tokenStore().apply { write(signedIn) }
            val reached = CompletableDeferred<Unit>()
            val answer = CompletableDeferred<Unit>()
            val tenancy =
                repository(store) {
                    reached.complete(Unit)
                    answer.await()
                    respondJson(switchedSession(refreshToken = "refresh-switched"))
                }

            val screen = launch { tenancy.switchTo(MOTO_PECAS.id) }
            reached.await()
            screen.cancel()
            answer.complete(Unit)
            screen.join() // DataStore writes on a real IO thread

            assertEquals(SessionTokens("access-switched", "refresh-switched"), store.read()?.tokens)
        }

    @Test
    fun aLogoutDuringASwitchStaysALogout() =
        runTest {
            val store = tokenStore().apply { write(signedIn) }
            val reached = CompletableDeferred<Unit>()
            val answer = CompletableDeferred<Unit>()
            val tenancy =
                repository(store) {
                    reached.complete(Unit)
                    answer.await()
                    respondJson(switchedSession(refreshToken = "refresh-switched"))
                }

            val switch = async { runCatching { tenancy.switchTo(MOTO_PECAS.id) } }
            reached.await()
            store.clear() // Sair, while the switch is on the wire
            answer.complete(Unit)

            assertTrue(switch.await().exceptionOrNull() is Failure)
            assertNull(store.read())
        }

    @Test
    fun aSwitchWaitsForARefreshInFlightAndPresentsItsNewToken() =
        runTest {
            // Both spend the refresh token: presenting the old one twice would leave one of them with a
            // superseded token, and its next use would sign the user out (ADR 0013).
            val store = tokenStore().apply { write(signedIn) }
            val refreshing = CompletableDeferred<Unit>()
            val refreshMayAnswer = CompletableDeferred<Unit>()
            val switchBodies = mutableListOf<String>()
            val turns = SessionTurns()
            val tenancy =
                repository(store, turns) { request ->
                    when (request.url.encodedPath) {
                        "/auth/refresh" -> {
                            refreshing.complete(Unit)
                            refreshMayAnswer.await()
                            respondJson(refreshedSession())
                        }
                        "/auth/switch-tenant" -> {
                            switchBodies += request.body.toByteArray().decodeToString()
                            respondJson(switchedSession(refreshToken = "refresh-switched"))
                        }
                        else ->
                            if (request.headers[HttpHeaders.Authorization] == "Bearer access-old") {
                                respondProblem(
                                    HttpStatusCode.Unauthorized,
                                    """{"status":401,"code":"unauthenticated"}""",
                                )
                            } else {
                                respondJson("{}")
                            }
                    }
                }
            launch { tenancy.client.get("/api/v1/admin/orders") }
            refreshing.await()
            val switch = async { tenancy.repository.switchTo(MOTO_PECAS.id) }
            // Real time, beyond the test scheduler: a switch that didn't wait its turn would go out now.
            withContext(Dispatchers.Default) { delay(SETTLE_MILLIS) }
            refreshMayAnswer.complete(Unit)
            switch.await()

            assertEquals(listOf("""{"tenantId":"${MOTO_PECAS.id}","refreshToken":"refresh-refreshed"}"""), switchBodies)
        }

    @Test
    fun createsAStore() =
        runTest {
            val tenancy =
                repository(tokenStore().apply { write(signedIn) }) {
                    respond(
                        """{"id":"${MOTO_PECAS.id}","name":"Moto Peças","slug":"moto-pecas",""" +
                            """"createdAt":"2026-11-25T12:00:00Z"}""",
                        HttpStatusCode.Created,
                        headersOf(HttpHeaders.ContentType, "application/json"),
                    )
                }

            assertEquals(MOTO_PECAS, tenancy.createStore(name = "Moto Peças", slug = "moto-pecas"))
        }

    @Test
    fun aTakenLinkNamesTheField() =
        runTest {
            val tenancy =
                repository(tokenStore().apply { write(signedIn) }) {
                    respondProblem(
                        HttpStatusCode.Conflict,
                        """{"title":"Link em uso","status":409,"code":"slug_taken"}""",
                    )
                }

            val failure =
                assertFailsWith<Failure.AlreadyTaken> { tenancy.createStore(name = "Moto Peças", slug = "moto-pecas") }

            assertEquals("slug", failure.field)
        }

    private fun TestScope.tokenStore(): TokenStore =
        TokenStore(
            dataStore = temporaryPreferencesDataStore(backgroundScope),
            cipher = FakeSecretCipher(),
            cryptoDispatcher = StandardTestDispatcher(testScheduler),
        )

    private fun repository(
        store: TokenStore,
        handler: MockRequestHandler,
    ): ApiTenancyRepository = repository(store, SessionTurns(), handler).repository

    /** The repository and the client it uses, so a test can make other calls on the same session. */
    private class Wired(
        val repository: ApiTenancyRepository,
        val client: HttpClient,
    )

    private fun repository(
        store: TokenStore,
        turns: SessionTurns,
        handler: MockRequestHandler,
    ): Wired {
        val client =
            createSessionApiClient(MockEngine(handler), ApiClientConfig(baseUrl = "https://api.test"), store, turns)
        return Wired(ApiTenancyRepository(TenancyApi(client), SessionKeeper(AuthApi(client), store, turns)), client)
    }

    private fun MockRequestHandleScope.respondJson(body: String): HttpResponseData =
        respond(body, HttpStatusCode.OK, headersOf(HttpHeaders.ContentType, "application/json"))

    private fun MockRequestHandleScope.respondProblem(
        status: HttpStatusCode,
        body: String,
    ): HttpResponseData = respond(body, status, headersOf(HttpHeaders.ContentType, "application/problem+json"))

    private fun switchedSession(refreshToken: String): String =
        """
        {"accessToken": "access-switched", "tokenType": "Bearer", "expiresIn": 900, "refreshToken": "$refreshToken",
         "user": {"id": "0190f3c4-0000-7000-8000-0000000000aa", "name": "Ana Souza", "email": "ana@altarpm.com.br"},
         "tenant": {"id": "${MOTO_PECAS.id}", "name": "Moto Peças", "slug": "moto-pecas"},
         "role": "staff", "permissions": ["order:read"],
         "memberships": [
           {"tenant": {"id": "${AltaRpm.id}", "name": "Alta RPM", "slug": "alta-rpm"}, "role": "owner"},
           {"tenant": {"id": "${MOTO_PECAS.id}", "name": "Moto Peças", "slug": "moto-pecas"}, "role": "staff"}
         ]}
        """

    private fun refreshedSession(): String =
        """
        {"accessToken": "access-refreshed", "tokenType": "Bearer", "expiresIn": 900, "refreshToken": "refresh-refreshed",
         "user": {"id": "0190f3c4-0000-7000-8000-0000000000aa", "name": "Ana Souza", "email": "ana@altarpm.com.br"},
         "tenant": {"id": "${AltaRpm.id}", "name": "Alta RPM", "slug": "alta-rpm"},
         "role": "owner", "permissions": [],
         "memberships": [{"tenant": {"id": "${AltaRpm.id}", "name": "Alta RPM", "slug": "alta-rpm"}, "role": "owner"}]}
        """

    private companion object {
        val MOTO_PECAS =
            TenantSummary(id = "0190f3c4-0000-7000-8000-000000000002", name = "Moto Peças", slug = "moto-pecas")

        /** Long enough for a switch that skipped the turn to reach the server first. */
        const val SETTLE_MILLIS = 200L
    }
}
