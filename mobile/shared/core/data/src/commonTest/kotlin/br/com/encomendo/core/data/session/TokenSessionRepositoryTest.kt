package br.com.encomendo.core.data.session

import app.cash.turbine.test
import br.com.encomendo.core.network.ApiClientConfig
import br.com.encomendo.core.network.SessionTurns
import br.com.encomendo.core.network.auth.AuthApi
import br.com.encomendo.core.security.SessionTokens
import br.com.encomendo.core.security.StoredSession
import br.com.encomendo.core.security.TokenStore
import br.com.encomendo.core.testing.FakeSecretCipher
import br.com.encomendo.core.testing.temporaryPreferencesDataStore
import br.com.encomendo.core.testing.testSession
import io.ktor.client.engine.mock.MockEngine
import io.ktor.client.engine.mock.MockRequestHandler
import io.ktor.client.engine.mock.respond
import io.ktor.http.HttpStatusCode
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

class TokenSessionRepositoryTest {
    @Test
    fun sessionFollowsTheTokenStore() =
        runTest {
            val store =
                TokenStore(
                    dataStore = temporaryPreferencesDataStore(backgroundScope),
                    cipher = FakeSecretCipher(),
                    cryptoDispatcher = StandardTestDispatcher(testScheduler),
                )
            val sessions = TokenSessionRepository(store, keeper(store) { error("observing asks nothing") })
            val session = testSession(permissions = setOf("order:read"))

            sessions.session.test {
                assertNull(awaitItem())
                store.write(StoredSession(SessionTokens(accessToken = "access", refreshToken = "refresh"), session))
                assertEquals(session, awaitItem())
                store.clear()
                assertNull(awaitItem())
            }
        }

    @Test
    fun renewingAsksTheServerForTheStoredSession() =
        runTest {
            val store =
                TokenStore(
                    dataStore = temporaryPreferencesDataStore(backgroundScope),
                    cipher = FakeSecretCipher(),
                    cryptoDispatcher = StandardTestDispatcher(testScheduler),
                )
            store.write(StoredSession(SessionTokens(accessToken = "access", refreshToken = "refresh"), testSession()))
            val paths = mutableListOf<String>()
            val sessions =
                TokenSessionRepository(
                    store,
                    keeper(store) { request ->
                        paths += request.url.encodedPath
                        respond("", HttpStatusCode.TooManyRequests) // settled: no retry
                    },
                )

            sessions.renew()

            assertEquals(listOf("/auth/refresh"), paths)
        }

    private fun keeper(
        store: TokenStore,
        handler: MockRequestHandler,
    ): SessionKeeper {
        val turns = SessionTurns()
        val client =
            createSessionApiClient(MockEngine(handler), ApiClientConfig(baseUrl = "https://api.test"), store, turns)
        return SessionKeeper(AuthApi(client), store, turns)
    }
}
