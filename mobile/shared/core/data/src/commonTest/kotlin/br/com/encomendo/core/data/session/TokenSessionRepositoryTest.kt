package br.com.encomendo.core.data.session

import app.cash.turbine.test
import br.com.encomendo.core.security.SessionTokens
import br.com.encomendo.core.security.TokenStore
import br.com.encomendo.core.testing.FakeSecretCipher
import br.com.encomendo.core.testing.temporaryPreferencesDataStore
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertFalse
import kotlin.test.assertTrue

class TokenSessionRepositoryTest {
    @Test
    fun signedInFollowsTheTokenStore() =
        runTest {
            val store =
                TokenStore(
                    dataStore = temporaryPreferencesDataStore(backgroundScope),
                    cipher = FakeSecretCipher(),
                    cryptoDispatcher = StandardTestDispatcher(testScheduler),
                )
            val sessions = TokenSessionRepository(store)

            sessions.signedIn.test {
                assertFalse(awaitItem())
                store.write(SessionTokens(accessToken = "access", refreshToken = "refresh"))
                assertTrue(awaitItem())
                store.clear()
                assertFalse(awaitItem())
            }
        }
}
