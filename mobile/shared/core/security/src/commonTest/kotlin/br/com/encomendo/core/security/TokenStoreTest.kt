package br.com.encomendo.core.security

import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import app.cash.turbine.test
import br.com.encomendo.core.testing.FakeSecretCipher
import br.com.encomendo.core.testing.temporaryPreferencesDataStore
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

class TokenStoreTest {
    private val session = SessionTokens(accessToken = "access-secret", refreshToken = "refresh-secret")
    private val other = SessionTokens(accessToken = "access-other", refreshToken = "refresh-other")

    @Test
    fun startsSignedOut() =
        runTest {
            val store = tokenStore()

            assertNull(store.read())
            assertFalse(store.signedIn.first())
        }

    @Test
    fun readsBackWhatItWroteEvenFromAnotherInstance() =
        runTest {
            val dataStore = temporaryPreferencesDataStore(backgroundScope)

            tokenStore(dataStore).write(session)

            assertEquals(session, tokenStore(dataStore).read(), "the session must survive an app restart")
        }

    @Test
    fun keepsOnlySealedBytesInStorage() =
        runTest {
            // Mobile rule: never put a token in plain DataStore (mobile/CLAUDE.md, Auth and offline).
            val dataStore = temporaryPreferencesDataStore(backgroundScope)

            tokenStore(dataStore).write(session)

            val stored =
                dataStore.data
                    .first()
                    .asMap()
                    .values
            assertEquals(1, stored.size)
            val raw = (stored.single() as ByteArray).decodeToString()
            assertFalse(raw.contains("secret"), "plain token in storage: $raw")
        }

    @Test
    fun clearSignsOut() =
        runTest {
            val store = tokenStore()
            store.write(session)

            store.clear()

            assertNull(store.read())
            assertFalse(store.signedIn.first())
        }

    @Test
    fun signedInFollowsLoginsAndLogouts() =
        runTest {
            val store = tokenStore()

            store.signedIn.test {
                assertFalse(awaitItem())
                store.write(session)
                assertTrue(awaitItem())
                store.write(other) // a refresh or tenant switch is not a new sign-in
                store.clear()
                assertFalse(awaitItem())
            }
        }

    @Test
    fun replaceSwapsOnlyTheExpectedSession() =
        runTest {
            val store = tokenStore()
            store.write(session)

            assertFalse(store.replace(expected = other, replacement = null), "another session must stay")
            assertEquals(session, store.read())

            assertTrue(store.replace(expected = session, replacement = other))
            assertEquals(other, store.read())

            assertTrue(store.replace(expected = other, replacement = null))
            assertNull(store.read())
            assertFalse(store.replace(expected = other, replacement = session), "a logout must stay")
            assertNull(store.read())
        }

    @Test
    fun aSessionTheDeviceCannotDecryptIsSignedOut() =
        runTest {
            // The Keystore key can disappear (device restore, cleared credentials): the user logs in again.
            val dataStore = temporaryPreferencesDataStore(backgroundScope)
            tokenStore(dataStore).write(session)

            val lostKey = tokenStore(dataStore, FakeSecretCipher(canOpen = false))

            assertNull(lostKey.read())
            assertFalse(lostKey.signedIn.first())
        }

    @Test
    fun onlyOneOfTwoRacingReplacesWins() =
        runTest {
            // Two refreshes answered at once: a read-then-write replace would let both "win".
            val third = SessionTokens(accessToken = "access-third", refreshToken = "refresh-third")
            val store = tokenStore()
            store.write(session)

            val results =
                listOf(other, third)
                    .map { async { store.replace(expected = session, replacement = it) } }
                    .awaitAll()

            assertEquals(1, results.count { it })
            assertTrue(store.read() in listOf(other, third))
        }

    @Test
    fun aLogoutRacingARefreshNeverSignsTheUserBackIn() =
        runTest {
            val store = tokenStore()
            store.write(session)

            val refresh = async { store.replace(expected = session, replacement = other) }
            store.clear()
            refresh.await()

            assertNull(store.read())
        }

    @Test
    fun aBlobThatDecryptsToGarbageIsSignedOut() =
        runTest {
            val dataStore = temporaryPreferencesDataStore(backgroundScope)
            tokenStore(dataStore).write(session)
            val garbage =
                object : SecretCipher by FakeSecretCipher() {
                    override fun open(sealed: ByteArray): ByteArray = "not a session".encodeToByteArray()
                }

            assertNull(tokenStore(dataStore, garbage).read())
        }

    @Test
    fun decryptsOnlyWhenTheStoredSessionChanges() =
        runTest {
            // Every API request reads the tokens; each Keystore call costs a few milliseconds.
            val cipher = CountingCipher()
            val store = tokenStore(cipher = cipher)
            store.write(session)

            repeat(3) { assertEquals(session, store.read()) }
            store.write(other)
            assertEquals(other, store.read())

            assertEquals(2, cipher.opens)
        }

    @Test
    fun neverPrintsTheTokens() {
        assertFalse(session.toString().contains("secret"), session.toString())
    }

    /** Counts decryptions; runs on the test dispatcher only, so a plain counter is enough. */
    private class CountingCipher(
        private val inner: SecretCipher = FakeSecretCipher(),
    ) : SecretCipher by inner {
        var opens = 0
            private set

        override fun open(sealed: ByteArray): ByteArray? {
            opens++
            return inner.open(sealed)
        }
    }

    private fun TestScope.tokenStore(
        dataStore: DataStore<Preferences> = temporaryPreferencesDataStore(backgroundScope),
        cipher: SecretCipher = FakeSecretCipher(),
    ): TokenStore = TokenStore(dataStore, cipher, cryptoDispatcher = StandardTestDispatcher(testScheduler))
}
