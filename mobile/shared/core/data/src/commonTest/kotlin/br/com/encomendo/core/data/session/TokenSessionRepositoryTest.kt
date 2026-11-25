package br.com.encomendo.core.data.session

import app.cash.turbine.test
import br.com.encomendo.core.security.SessionTokens
import br.com.encomendo.core.security.StoredSession
import br.com.encomendo.core.security.TokenStore
import br.com.encomendo.core.testing.FakeSecretCipher
import br.com.encomendo.core.testing.temporaryPreferencesDataStore
import br.com.encomendo.core.testing.testSession
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
            val sessions = TokenSessionRepository(store)
            val session = testSession(permissions = setOf("order:read"))

            sessions.session.test {
                assertNull(awaitItem())
                store.write(StoredSession(SessionTokens(accessToken = "access", refreshToken = "refresh"), session))
                assertEquals(session, awaitItem())
                store.clear()
                assertNull(awaitItem())
            }
        }
}
