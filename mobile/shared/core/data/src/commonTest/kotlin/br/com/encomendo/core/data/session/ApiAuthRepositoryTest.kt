package br.com.encomendo.core.data.session

import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.emptyPreferences
import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.domain.failure.InputIssue
import br.com.encomendo.core.domain.session.GrantedPermissions
import br.com.encomendo.core.domain.session.Permission
import br.com.encomendo.core.network.ApiClientConfig
import br.com.encomendo.core.network.SessionTurns
import br.com.encomendo.core.network.auth.AuthApi
import br.com.encomendo.core.security.SecretCipher
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
import io.ktor.client.request.HttpRequestData
import io.ktor.client.request.HttpResponseData
import io.ktor.http.HttpHeaders
import io.ktor.http.HttpStatusCode
import io.ktor.http.headersOf
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.yield
import kotlinx.io.IOException
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNull
import kotlin.test.assertTrue

@OptIn(ExperimentalCoroutinesApi::class) // advanceUntilIdle
class ApiAuthRepositoryTest {
    @Test
    fun aSignUpKeepsTheNewAccountsSessionWithoutAStore() =
        runTest {
            // The root then opens the store picker: "Crie sua loja" (ADR 0010).
            val store = tokenStore()
            val auth = repository(store) { respondJson(NO_STORE_SESSION_JSON, HttpStatusCode.Created) }

            val session = auth.signUp(name = "Ana Souza", email = "ana@altarpm.com.br", password = "segredo-forte")

            assertNull(session.tenant)
            assertEquals(SessionTokens(accessToken = "access-new", refreshToken = "refresh-new"), store.read()?.tokens)
            assertEquals(session, store.read()?.session)
        }

    @Test
    fun aSignUpTheScreenGaveUpOnStillKeepsTheNewAccount() =
        runTest {
            // Back while "Criando conta…": the server may have created the account already. Dropping its
            // session would leave the user locked out of a retry ("Já existe uma conta com este e-mail.").
            val store = tokenStore()
            val reached = CompletableDeferred<Unit>()
            val answer = CompletableDeferred<Unit>()
            val auth =
                repository(store) {
                    reached.complete(Unit)
                    answer.await()
                    respondJson(NO_STORE_SESSION_JSON, HttpStatusCode.Created)
                }

            val screen =
                launch { auth.signUp(name = "Ana Souza", email = "ana@altarpm.com.br", password = "segredo-forte") }
            reached.await()
            screen.cancel()
            answer.complete(Unit)
            screen.join() // DataStore writes on a real IO thread

            assertEquals("refresh-new", store.read()?.tokens?.refreshToken)
        }

    @Test
    fun anEmailInUseIsMarkedOnTheEmailField() =
        runTest {
            val store = tokenStore()
            val auth =
                repository(store) {
                    respondProblem(
                        HttpStatusCode.Conflict,
                        """{"title":"E-mail em uso","status":409,"code":"email_taken"}""",
                    )
                }

            val failure =
                assertFailsWith<Failure.AlreadyTaken> { auth.signUp("Ana", "ana@altarpm.com.br", "segredo-forte") }

            assertEquals("email", failure.field)
            assertNull(store.read())
        }

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
    fun aLoginAsksForTheStoreUsedLastOnThisDevice() =
        runTest {
            val lastStore = LastStore(temporaryPreferencesDataStore(backgroundScope))
            lastStore.follow(flowOf(testSession(tenant = AltaRpm))) // a finite flow: returns once remembered
            val bodies = mutableListOf<String>()
            val auth =
                repository(tokenStore(), lastStore) { request ->
                    bodies += request.body.toByteArray().decodeToString()
                    respondJson(SESSION_JSON)
                }

            auth.logIn(email = "ana@altarpm.com.br", password = "segredo-forte")

            assertTrue(bodies.single().contains(""""tenantId":"${AltaRpm.id}""""), bodies.single())
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

    @Test
    fun aLogoutForgetsTheSessionAtOnce() =
        runTest {
            // In the workshop the signal may be gone: leaving must never wait on the server (ADR 0016).
            val store = tokenStore().apply { write(signedIn) }
            val server = CompletableDeferred<Unit>()
            val auth = repository(store) { server.await().let { respond("", HttpStatusCode.NoContent) } }

            auth.logOut()

            assertNull(store.read())
        }

    @Test
    fun aLogoutAsksTheServerToRevokeTheTokensItForgot() =
        runTest {
            val store = tokenStore().apply { write(signedIn) }
            val seen = CompletableDeferred<HttpRequestData>()
            val auth =
                repository(store) { request ->
                    seen.complete(request)
                    respond("", HttpStatusCode.NoContent)
                }

            auth.logOut()

            val request = seen.await()
            assertEquals("/auth/logout", request.url.encodedPath)
            assertEquals("Bearer access-old", request.headers[HttpHeaders.Authorization])
            assertEquals("""{"refreshToken":"refresh-old"}""", request.body.toByteArray().decodeToString())
        }

    @Test
    fun aLogoutEmptiesWhatTheDeviceCachedForThePersonLeaving() =
        runTest {
            // Signed photo URLs show their pictures to whoever holds the phone next (ADR 0016, ADR 0026).
            val store = tokenStore().apply { write(signedIn) }
            var cleared = 0
            val auth = repository(store, deviceData = { cleared++ }) { respond("", HttpStatusCode.NoContent) }

            auth.logOut()

            assertNull(store.read())
            assertEquals(1, cleared)
        }

    @Test
    fun aLogoutTheDeviceCannotSaveKeepsWhatItCached() =
        runTest {
            val disk = WriteFailing(temporaryPreferencesDataStore(backgroundScope))
            val store =
                TokenStore(disk, FakeSecretCipher(), StandardTestDispatcher(testScheduler))
            store.write(signedIn)
            disk.failing = true
            var cleared = 0
            val auth = repository(store, deviceData = { cleared++ }) { respond("", HttpStatusCode.NoContent) }

            assertFailsWith<Failure.Unexpected> { auth.logOut() }

            assertEquals(0, cleared, "the session on screen still works, so its pictures stay")
        }

    @Test
    fun aLogoutFinishesWhenTheScreenThatAskedIsGone() =
        runTest {
            // The root clears the signed-in screens, and so cancels the account bar that asked, as soon
            // as it sees the session end: while the clear is still on its way back to the caller.
            val disk = CancelsOnWrite(temporaryPreferencesDataStore(backgroundScope))
            val store =
                TokenStore(
                    dataStore = disk,
                    cipher = FakeSecretCipher(),
                    cryptoDispatcher = StandardTestDispatcher(testScheduler),
                )
            store.write(signedIn)
            val seen = CompletableDeferred<HttpRequestData>()
            val auth =
                repository(store) { request ->
                    seen.complete(request)
                    respond("", HttpStatusCode.NoContent)
                }
            lateinit var screen: Job
            disk.onWrite = { screen.cancel() }

            screen = launch { auth.logOut() }
            screen.join() // DataStore writes its file on a real IO thread, beyond the test scheduler

            assertTrue(screen.isCancelled, "the caller was cancelled before logOut returned")
            assertEquals("/auth/logout", seen.await().url.encodedPath)
            assertNull(store.read())
        }

    @Test
    fun anOfflineLogoutStillSignsOut() =
        runTest {
            val store = tokenStore().apply { write(signedIn) }
            val auth = repository(store) { throw IOException("no signal") }

            auth.logOut()
            advanceUntilIdle()

            assertNull(store.read())
        }

    @Test
    fun loggingOutWhenSignedOutCallsNobody() =
        runTest {
            val calls = mutableListOf<String>()
            val auth =
                repository(tokenStore()) { request ->
                    calls += request.url.encodedPath
                    respond("", HttpStatusCode.NoContent)
                }

            auth.logOut()
            advanceUntilIdle()

            assertEquals(emptyList(), calls)
        }

    @Test
    fun aLogoutTheDeviceCannotSaveIsAFailureAndCallsNobody() =
        runTest {
            // Telling the server first would revoke a session the device still shows as signed in.
            val disk = WriteFailing(temporaryPreferencesDataStore(backgroundScope))
            val store =
                TokenStore(
                    dataStore = disk,
                    cipher = FakeSecretCipher(),
                    cryptoDispatcher = StandardTestDispatcher(testScheduler),
                )
            store.write(signedIn)
            disk.failing = true
            val calls = mutableListOf<String>()
            val auth =
                repository(store) { request ->
                    calls += request.url.encodedPath
                    respond("", HttpStatusCode.NoContent)
                }

            val failure = assertFailsWith<Failure.Unexpected> { auth.logOut() }
            advanceUntilIdle()

            assertEquals(0, failure.status)
            assertEquals(emptyList(), calls)
            assertEquals(signedIn, store.read(), "the session on screen still works")
        }

    private val signedIn = StoredSession(SessionTokens("access-old", "refresh-old"), testSession())

    private fun TestScope.tokenStore(): TokenStore =
        TokenStore(
            dataStore = temporaryPreferencesDataStore(backgroundScope),
            cipher = FakeSecretCipher(),
            cryptoDispatcher = StandardTestDispatcher(testScheduler),
        )

    private fun TestScope.repository(
        store: TokenStore,
        lastStore: LastStore = LastStore(temporaryPreferencesDataStore(backgroundScope)),
        deviceData: DeviceDataCleaner = DeviceDataCleaner {},
        handler: MockRequestHandler,
    ): ApiAuthRepository =
        ApiAuthRepository(
            api =
                AuthApi(
                    createSessionApiClient(
                        MockEngine(handler),
                        ApiClientConfig(baseUrl = "https://api.test"),
                        store,
                        SessionTurns(),
                    ),
                ),
            store = store,
            appScope = backgroundScope,
            lastStore = lastStore,
            deviceData = deviceData,
        )

    private fun MockRequestHandleScope.respondJson(
        body: String,
        status: HttpStatusCode = HttpStatusCode.OK,
    ): HttpResponseData = respond(body, status, headersOf(HttpHeaders.ContentType, "application/json"))

    private fun MockRequestHandleScope.respondProblem(
        status: HttpStatusCode,
        body: String,
    ): HttpResponseData = respond(body, status, headersOf(HttpHeaders.ContentType, "application/problem+json"))

    /** A cipher that can open nothing and seal nothing, as a broken Keystore. */
    private object BrokenKeystore : SecretCipher {
        override fun seal(plain: ByteArray): ByteArray = throw IOException("the Keystore could not seal the session")

        override fun open(sealed: ByteArray): ByteArray? = null
    }

    /** Passes everything to [delegate] until [failing] is set; then every write fails, as on a full disk. */
    private class WriteFailing(
        private val delegate: DataStore<Preferences>,
    ) : DataStore<Preferences> {
        var failing = false

        override val data: Flow<Preferences> = delegate.data

        override suspend fun updateData(transform: suspend (t: Preferences) -> Preferences): Preferences =
            if (failing) throw IOException("no space left on device") else delegate.updateData(transform)
    }

    /**
     * Passes everything to [delegate]; after each write it runs [onWrite], then suspends before
     * returning, as the root's session collector runs before the writer resumes on a device.
     */
    private class CancelsOnWrite(
        private val delegate: DataStore<Preferences>,
    ) : DataStore<Preferences> {
        var onWrite: () -> Unit = {}

        override val data: Flow<Preferences> = delegate.data

        override suspend fun updateData(transform: suspend (t: Preferences) -> Preferences): Preferences =
            delegate.updateData(transform).also {
                onWrite()
                yield()
            }
    }

    /** A DataStore whose every write fails, as on a full disk. */
    private object FullDisk : DataStore<Preferences> {
        override val data: Flow<Preferences> = flowOf(emptyPreferences())

        override suspend fun updateData(transform: suspend (t: Preferences) -> Preferences): Preferences =
            throw IOException("no space left on device")
    }

    private companion object {
        const val NO_STORE_SESSION_JSON = """
            {
              "accessToken": "access-new",
              "tokenType": "Bearer",
              "expiresIn": 900,
              "refreshToken": "refresh-new",
              "user": {"id": "0190f3c4-0000-7000-8000-0000000000aa", "name": "Ana Souza", "email": "ana@altarpm.com.br"},
              "permissions": [],
              "memberships": []
            }
        """

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
