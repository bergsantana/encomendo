package br.com.encomendo.core.data.session

import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.emptyPreferences
import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.domain.failure.InputIssue
import br.com.encomendo.core.domain.session.GrantedPermissions
import br.com.encomendo.core.domain.session.Permission
import br.com.encomendo.core.network.ApiClientConfig
import br.com.encomendo.core.network.auth.AuthApi
import br.com.encomendo.core.security.SecretCipher
import br.com.encomendo.core.security.SessionTokens
import br.com.encomendo.core.security.StoredSession
import br.com.encomendo.core.security.TokenStore
import br.com.encomendo.core.testing.FakeSecretCipher
import br.com.encomendo.core.testing.temporaryPreferencesDataStore
import br.com.encomendo.core.testing.testSession
import io.ktor.client.engine.mock.MockEngine
import io.ktor.client.engine.mock.MockRequestHandleScope
import io.ktor.client.engine.mock.MockRequestHandler
import io.ktor.client.engine.mock.respond
import io.ktor.client.request.HttpResponseData
import io.ktor.http.HttpHeaders
import io.ktor.http.HttpStatusCode
import io.ktor.http.headersOf
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runTest
import kotlinx.io.IOException
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNull

class ApiAuthRepositoryTest {
    @Test
    fun aLoginKeepsTheWholeSessionWithItsTokens() =
        runTest {
            val store = tokenStore()
            val auth = repository(store) { respondJson(SESSION_JSON) }

            val session = auth.logIn(email = "ana@altarpm.com.br", password = "segredo-forte")

            val stored = store.read()
            assertEquals(SessionTokens(accessToken = "access-new", refreshToken = "refresh-new"), stored?.tokens)
            assertEquals(session, stored?.session)
            assertEquals("alta-rpm", session.tenant?.slug)
            assertEquals(GrantedPermissions(listOf(Permission("order:read"))), session.permissions)
        }

    @Test
    fun aLoginReplacesASessionLeftOnTheDevice() =
        runTest {
            val store =
                tokenStore().apply {
                    write(
                        StoredSession(SessionTokens("access-old", "refresh-old"), testSession(userId = "someone-else")),
                    )
                }
            val auth = repository(store) { respondJson(SESSION_JSON) }

            auth.logIn(email = "ana@altarpm.com.br", password = "segredo-forte")

            assertEquals("refresh-new", store.read()?.tokens?.refreshToken)
        }

    @Test
    fun wrongCredentialsAreAFormErrorAndStoreNothing() =
        runTest {
            val store = tokenStore()
            val auth =
                repository(store) {
                    respondProblem(
                        HttpStatusCode.Unauthorized,
                        """{"title":"E-mail ou senha incorretos","status":401,"code":"invalid_credentials"}""",
                    )
                }

            assertFailsWith<Failure.WrongCredentials> { auth.logIn(email = "ana@altarpm.com.br", password = "errada") }
            assertNull(store.read())
        }

    @Test
    fun invalidInputListsTheFieldsInTheServersWords() =
        runTest {
            val auth =
                repository(tokenStore()) {
                    respondProblem(
                        HttpStatusCode.BadRequest,
                        """{"title":"Dados inválidos","status":400,"code":"invalid",
                           "errors":[{"field":"email","message":"Informe um e-mail válido."}]}""",
                    )
                }

            val failure =
                assertFailsWith<Failure.InvalidInput> { auth.logIn(email = "ana", password = "segredo-forte") }

            assertEquals(listOf(InputIssue(field = "email", message = "Informe um e-mail válido.")), failure.issues)
        }

    @Test
    fun aRateLimitCarriesTheWait() =
        runTest {
            val auth =
                repository(tokenStore()) {
                    respond(
                        """{"title":"Muitas tentativas","status":429,"code":"rate_limited"}""",
                        HttpStatusCode.TooManyRequests,
                        headersOf(
                            HttpHeaders.ContentType to listOf("application/problem+json"),
                            HttpHeaders.RetryAfter to listOf("30"),
                        ),
                    )
                }

            val failure =
                assertFailsWith<Failure.RateLimited> { auth.logIn(email = "ana@altarpm.com.br", password = "x") }

            assertEquals(30, failure.retryAfterSeconds)
        }

    @Test
    fun noSignalIsOffline() =
        runTest {
            val auth = repository(tokenStore()) { throw IOException("connection reset") }

            assertFailsWith<Failure.Offline> { auth.logIn(email = "ana@altarpm.com.br", password = "segredo-forte") }
        }

    @Test
    fun aServerFailureIsUnexpected() =
        runTest {
            val auth =
                repository(tokenStore()) {
                    respondProblem(
                        HttpStatusCode.ServiceUnavailable,
                        """{"title":"Indisponível","status":503,"code":"not_ready"}""",
                    )
                }

            val failure =
                assertFailsWith<Failure.Unexpected> { auth.logIn(email = "ana@altarpm.com.br", password = "x") }

            assertEquals(503, failure.status)
            assertEquals("not_ready", failure.code)
        }

    @Test
    fun aSessionTheDeviceCannotSaveIsAFailure() =
        runTest {
            // The login worked on the server, but the device can't keep it: the user must see an
            // error and stay on the login screen, not crash or look signed in.
            val store =
                TokenStore(
                    dataStore = FullDisk,
                    cipher = FakeSecretCipher(),
                    cryptoDispatcher = StandardTestDispatcher(testScheduler),
                )
            val auth = repository(store) { respondJson(SESSION_JSON) }

            val failure =
                assertFailsWith<Failure.Unexpected> { auth.logIn(email = "ana@altarpm.com.br", password = "x") }

            assertEquals(0, failure.status)
        }

    @Test
    fun aSessionTheKeystoreCannotSealIsAFailure() =
        runTest {
            // Some devices' Keystores fail after an update; KeystoreCipher reports that as IOException.
            val store =
                TokenStore(
                    dataStore = temporaryPreferencesDataStore(backgroundScope),
                    cipher = BrokenKeystore,
                    cryptoDispatcher = StandardTestDispatcher(testScheduler),
                )
            val auth = repository(store) { respondJson(SESSION_JSON) }

            val failure =
                assertFailsWith<Failure.Unexpected> { auth.logIn(email = "ana@altarpm.com.br", password = "x") }

            assertEquals(0, failure.status)
            assertNull(store.read())
        }

    @Test
    fun failuresNeverPrintTheCredentials() =
        runTest {
            val auth =
                repository(tokenStore()) {
                    respondProblem(
                        HttpStatusCode.Unauthorized,
                        """{"title":"E-mail ou senha incorretos","status":401,"code":"invalid_credentials"}""",
                    )
                }

            val failure =
                assertFailsWith<Failure> { auth.logIn(email = "ana@altarpm.com.br", password = "segredo-forte") }

            val printed = failure.stackTraceToString()
            listOf("ana@altarpm.com.br", "segredo-forte").forEach { assertEquals(false, printed.contains(it), printed) }
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
    ): ApiAuthRepository =
        ApiAuthRepository(
            api =
                AuthApi(
                    createSessionApiClient(MockEngine(handler), ApiClientConfig(baseUrl = "https://api.test"), store),
                ),
            store = store,
        )

    private fun MockRequestHandleScope.respondJson(body: String): HttpResponseData =
        respond(body, HttpStatusCode.OK, headersOf(HttpHeaders.ContentType, "application/json"))

    private fun MockRequestHandleScope.respondProblem(
        status: HttpStatusCode,
        body: String,
    ): HttpResponseData = respond(body, status, headersOf(HttpHeaders.ContentType, "application/problem+json"))

    /** A cipher that can open nothing and seal nothing, as a broken Keystore. */
    private object BrokenKeystore : SecretCipher {
        override fun seal(plain: ByteArray): ByteArray = throw IOException("the Keystore could not seal the session")

        override fun open(sealed: ByteArray): ByteArray? = null
    }

    /** A DataStore whose every write fails, as on a full disk. */
    private object FullDisk : DataStore<Preferences> {
        override val data: Flow<Preferences> = flowOf(emptyPreferences())

        override suspend fun updateData(transform: suspend (t: Preferences) -> Preferences): Preferences =
            throw IOException("no space left on device")
    }

    private companion object {
        const val SESSION_JSON = """
            {
              "accessToken": "access-new",
              "tokenType": "Bearer",
              "expiresIn": 900,
              "refreshToken": "refresh-new",
              "user": {"id": "0190f3c4-0000-7000-8000-0000000000aa", "name": "Ana Souza", "email": "ana@altarpm.com.br"},
              "tenant": {"id": "0190f3c4-0000-7000-8000-000000000001", "name": "Alta RPM", "slug": "alta-rpm"},
              "role": "owner",
              "permissions": ["order:read"],
              "memberships": [
                {"tenant": {"id": "0190f3c4-0000-7000-8000-000000000001", "name": "Alta RPM", "slug": "alta-rpm"}, "role": "owner"}
              ]
            }
        """
    }
}
