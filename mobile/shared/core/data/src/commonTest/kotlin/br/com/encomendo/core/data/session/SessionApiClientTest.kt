package br.com.encomendo.core.data.session

import br.com.encomendo.core.domain.session.GrantedPermissions
import br.com.encomendo.core.domain.session.Permission
import br.com.encomendo.core.network.ApiClientConfig
import br.com.encomendo.core.network.ApiError
import br.com.encomendo.core.network.SessionTurns
import br.com.encomendo.core.security.SessionTokens
import br.com.encomendo.core.security.StoredSession
import br.com.encomendo.core.security.TokenStore
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
import kotlinx.coroutines.async
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runTest
import kotlinx.io.IOException
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNull

class SessionApiClientTest {
    private val stored =
        StoredSession(
            tokens = SessionTokens(accessToken = "access-old", refreshToken = "refresh-old"),
            session = testSession(permissions = setOf("order:read")),
        )

    @Test
    fun aRefreshKeepsTheNewTokensWithTheNewPermissions() =
        runTest {
            // A role change reaches the app with the next refresh (ADR 0010): the permissions
            // on screen must move with the tokens, never lag behind them.
            val store = tokenStore().apply { write(stored) }
            val refreshBodies = mutableListOf<String>()
            val client =
                client(store) { request ->
                    when {
                        request.url.encodedPath == "/auth/refresh" -> {
                            refreshBodies += request.body.toByteArray().decodeToString()
                            respondJson(refreshedSession(permissions = """["order:read","order:write"]"""))
                        }
                        request.headers[HttpHeaders.Authorization] == "Bearer access-old" -> respondUnauthenticated()
                        else -> respondJson("""{"items":[]}""")
                    }
                }

            client.get("/api/v1/admin/orders")

            assertEquals(listOf("""{"refreshToken":"refresh-old"}"""), refreshBodies)
            val renewed = store.read()
            assertEquals(SessionTokens(accessToken = "access-new", refreshToken = "refresh-new"), renewed?.tokens)
            assertEquals(
                GrantedPermissions(listOf(Permission("order:read"), Permission("order:write"))),
                store.session.first()?.permissions,
            )
        }

    @Test
    fun aRefusedRefreshSignsOut() =
        runTest {
            val store = tokenStore().apply { write(stored) }
            val client = client(store) { respondUnauthenticated() }

            val error = assertFailsWith<ApiError> { client.get("/api/v1/admin/orders") }

            assertEquals(401, error.status)
            assertNull(store.read())
            assertNull(store.session.first())
        }

    @Test
    fun anOfflineRefreshKeepsTheSession() =
        runTest {
            // Workshops lose signal: no answer is not a refused token (ADR 0012).
            val store = tokenStore().apply { write(stored) }
            val client =
                client(store) { request ->
                    if (request.url.encodedPath == "/auth/refresh") throw IOException("no signal")
                    respondUnauthenticated()
                }

            assertFailsWith<ApiError> { client.get("/api/v1/admin/orders") }

            assertEquals(stored, store.read())
        }

    @Test
    fun aLogoutDuringARefreshStaysALogout() =
        runTest {
            // "Sair" while the refresh is on the wire: its answer must not sign the user back in.
            val store = tokenStore().apply { write(stored) }
            val started = CompletableDeferred<Unit>()
            val gate = CompletableDeferred<Unit>()
            val client =
                client(store) { request ->
                    if (request.url.encodedPath != "/auth/refresh") return@client respondUnauthenticated()
                    started.complete(Unit)
                    gate.await()
                    respondJson(refreshedSession(permissions = """["order:read"]"""))
                }

            val call = async { assertFailsWith<ApiError> { client.get("/api/v1/admin/orders") } }
            started.await()
            store.clear()
            gate.complete(Unit)
            call.await()

            assertNull(store.read())
        }

    @Test
    fun aLoginDuringARefusedRefreshIsKept() =
        runTest {
            // The old session's refresh is refused after someone logged in again: the new session stays.
            val store = tokenStore().apply { write(stored) }
            val newLogin =
                StoredSession(
                    SessionTokens(accessToken = "access-login", refreshToken = "refresh-login"),
                    testSession(),
                )
            val started = CompletableDeferred<Unit>()
            val gate = CompletableDeferred<Unit>()
            val client =
                client(store) { request ->
                    if (request.url.encodedPath == "/auth/refresh") {
                        started.complete(Unit)
                        gate.await()
                    }
                    respondUnauthenticated()
                }

            val call = async { assertFailsWith<ApiError> { client.get("/api/v1/admin/orders") } }
            started.await()
            store.write(newLogin)
            gate.complete(Unit)
            call.await()

            assertEquals(newLogin, store.read())
        }

    @Test
    fun anUnusableRefreshAnswerFailsAsAnApiError() =
        runTest {
            // A 200 that breaks the contract must not escape as a decoding exception that no
            // screen handles. The token it spent is gone, so the next 401 ends the session.
            val store = tokenStore().apply { write(stored) }
            val client =
                client(store) { request ->
                    if (request.url.encodedPath == "/auth/refresh") return@client respondJson("""{"status":"ok"}""")
                    respondUnauthenticated()
                }

            val error = assertFailsWith<ApiError> { client.get("/api/v1/admin/orders") }

            assertEquals(200, error.status)
            assertEquals(stored, store.read())
        }

    private fun TestScope.tokenStore(): TokenStore =
        TokenStore(
            dataStore = temporaryPreferencesDataStore(backgroundScope),
            cipher = FakeSecretCipher(),
            cryptoDispatcher = StandardTestDispatcher(testScheduler),
        )

    private fun client(
        store: TokenStore,
        handler: MockRequestHandler,
    ): HttpClient =
        createSessionApiClient(
            MockEngine(handler),
            ApiClientConfig(baseUrl = "https://api.test"),
            store,
            SessionTurns(),
        )

    private fun MockRequestHandleScope.respondJson(body: String): HttpResponseData =
        respond(body, HttpStatusCode.OK, headersOf(HttpHeaders.ContentType, "application/json"))

    private fun MockRequestHandleScope.respondUnauthenticated(): HttpResponseData =
        respond(
            """{"type":"about:blank","title":"Sessão expirada","status":401,"code":"unauthenticated"}""",
            HttpStatusCode.Unauthorized,
            headersOf(HttpHeaders.ContentType, "application/problem+json"),
        )

    private fun refreshedSession(permissions: String): String =
        """
        {
          "accessToken": "access-new",
          "tokenType": "Bearer",
          "expiresIn": 900,
          "refreshToken": "refresh-new",
          "user": {"id": "0190f3c4-0000-7000-8000-0000000000aa", "name": "Ana Souza", "email": "ana@altarpm.com.br"},
          "tenant": {"id": "0190f3c4-0000-7000-8000-000000000001", "name": "Alta RPM", "slug": "alta-rpm"},
          "role": "admin",
          "permissions": $permissions,
          "memberships": [
            {"tenant": {"id": "0190f3c4-0000-7000-8000-000000000001", "name": "Alta RPM", "slug": "alta-rpm"}, "role": "admin"}
          ]
        }
        """
}
