package br.com.encomendo.core.security

import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.byteArrayPreferencesKey
import androidx.datastore.preferences.core.edit
import app.cash.turbine.test
import br.com.encomendo.core.domain.session.Membership
import br.com.encomendo.core.domain.session.Permission
import br.com.encomendo.core.domain.session.Role
import br.com.encomendo.core.domain.session.TenantSummary
import br.com.encomendo.core.testing.AltaRpm
import br.com.encomendo.core.testing.FakeSecretCipher
import br.com.encomendo.core.testing.temporaryPreferencesDataStore
import br.com.encomendo.core.testing.testSession
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue

class TokenStoreTest {
    private val session =
        StoredSession(
            tokens = SessionTokens(accessToken = "access-secret", refreshToken = "refresh-secret"),
            session = testSession(permissions = setOf("order:read")),
        )
    private val other =
        StoredSession(
            tokens = SessionTokens(accessToken = "access-other", refreshToken = "refresh-other"),
            session = testSession(permissions = setOf("order:read", "order:write")),
        )

    @Test
    fun startsSignedOut() =
        runTest {
            val store = tokenStore()

            assertNull(store.read())
            assertNull(store.session.first())
        }

    @Test
    fun readsBackWhatItWroteEvenFromAnotherInstance() =
        runTest {
            val dataStore = temporaryPreferencesDataStore(backgroundScope)

            tokenStore(dataStore).write(session)

            assertEquals(session, tokenStore(dataStore).read(), "the session must survive an app restart")
        }

    @Test
    fun keepsTheWholeSessionWithTheTokens() =
        runTest {
            // Offline, the cached session is all the app knows about the user's store and permissions.
            val dataStore = temporaryPreferencesDataStore(backgroundScope)

            tokenStore(dataStore).write(session)

            val read = tokenStore(dataStore).read()
            assertEquals(session.session, read?.session)
            assertEquals(session.session, tokenStore(dataStore).session.first())
        }

    @Test
    fun keepsWhichStoresAreSuspended() =
        runTest {
            // The picker marks them "Suspensa" offline too (ADR 0022).
            val store = tokenStore()
            val base = testSession(tenant = null)
            val suspended = base.copy(memberships = listOf(Membership(AltaRpm, Role("owner"), suspended = true)))

            store.write(StoredSession(session.tokens, suspended))

            assertEquals(
                listOf(true),
                store
                    .read()
                    ?.session
                    ?.memberships
                    ?.map { it.suspended },
            )
        }

    @Test
    fun keepsASessionWithoutAStore() =
        runTest {
            // Right after sign-up, or with several stores, no store is selected yet (ADR 0010).
            val store = tokenStore()
            val noStore = StoredSession(session.tokens, testSession(tenant = null))

            store.write(noStore)

            assertEquals(noStore, store.read())
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
            assertFalse(raw.contains("altarpm"), "plain email in storage: $raw")
        }

    @Test
    fun clearSignsOut() =
        runTest {
            val store = tokenStore()
            store.write(session)

            store.clear()

            assertNull(store.read())
            assertNull(store.session.first())
        }

    @Test
    fun sessionFollowsLoginsRefreshesAndLogouts() =
        runTest {
            val store = tokenStore()

            store.session.test {
                assertNull(awaitItem())
                store.write(session)
                assertEquals(session.session, awaitItem())
                store.write(other) // a refresh that brought a new permission
                assertEquals(other.session, awaitItem())
                store.write(StoredSession(session.tokens.copy(accessToken = "access-third"), other.session))
                // New tokens alone are not a new session: nothing to show.
                store.clear()
                assertNull(awaitItem())
            }
        }

    @Test
    fun aStoreSwitchEmitsTheNewStore() =
        runTest {
            val store = tokenStore()
            val elsewhere =
                TenantSummary(id = "0190f3c4-0000-7000-8000-000000000002", name = "Moto Peças", slug = "moto-pecas")
            store.write(session)

            store.session.test {
                assertEquals(session.session, awaitItem())
                store.write(StoredSession(other.tokens, testSession(tenant = elsewhere)))
                assertEquals(elsewhere, awaitItem()?.tenant)
            }
        }

    @Test
    fun replaceComparesOnlyTheTokens() =
        runTest {
            // The refresh token is unique, so the tokens alone say whether the session is still the
            // one that was refreshed; a change to what is stored with them must not drop rotated tokens.
            val store = tokenStore()
            store.write(session)
            store.write(StoredSession(session.tokens, other.session))

            assertTrue(store.replace(expected = session, replacement = other))
            assertEquals(other, store.read())
        }

    @Test
    fun readsTheStoredFormat() =
        runTest {
            // Renaming the key or a field signs every device out; this pins the format in use.
            val dataStore = temporaryPreferencesDataStore(backgroundScope)
            val blob =
                """
                {"access":"access-secret","refresh":"refresh-secret",
                 "user":{"id":"u-1","name":"Ana Souza","email":"ana@altarpm.com.br"},
                 "tenant":{"id":"t-1","name":"Alta RPM","slug":"alta-rpm"},"role":"owner",
                 "permissions":["order:read"],
                 "memberships":[{"tenant":{"id":"t-1","name":"Alta RPM","slug":"alta-rpm"},"role":"owner"}]}
                """.trimIndent()
            val sealed = FakeSecretCipher().seal(blob.encodeToByteArray())
            dataStore.edit { it[byteArrayPreferencesKey("session")] = sealed }

            val read = assertNotNull(tokenStore(dataStore).read())

            assertEquals(SessionTokens(accessToken = "access-secret", refreshToken = "refresh-secret"), read.tokens)
            assertEquals("ana@altarpm.com.br", read.session.user.email)
            val tenant = assertNotNull(read.session.tenant)
            assertEquals("alta-rpm", tenant.slug)
            assertEquals(Role("owner"), read.session.role)
            assertTrue(read.session.permissions.allows(Permission("order:read")))
            assertEquals(listOf(Membership(tenant, Role("owner"))), read.session.memberships)
        }

    @Test
    fun aMembershipStoredBeforeSuspensionsExistReadsAsNotSuspended() =
        runTest {
            // Written by an app version without the `suspended` key (ADR 0022): the device must stay
            // signed in, and no store reads as suspended until the next renewal says so.
            val dataStore = temporaryPreferencesDataStore(backgroundScope)
            val blob =
                """
                {"access":"a","refresh":"r","user":{"id":"u-1","name":"Ana Souza","email":"ana@altarpm.com.br"},
                 "memberships":[
                   {"tenant":{"id":"t-1","name":"Alta RPM","slug":"alta-rpm"},"role":"owner"},
                   {"tenant":{"id":"t-2","name":"Moto Peças","slug":"moto-pecas"},"role":"staff","suspended":true}]}
                """.trimIndent()
            dataStore.edit {
                it[
                    byteArrayPreferencesKey(
                        "session",
                    ),
                ] = FakeSecretCipher().seal(blob.encodeToByteArray())
            }

            val read = assertNotNull(tokenStore(dataStore).read())

            // The second row also pins the key's name: renaming it would hide every suspension.
            assertEquals(listOf(false, true), read.session.memberships.map { it.suspended })
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
            assertNull(lostKey.session.first())
        }

    @Test
    fun onlyOneOfTwoRacingReplacesWins() =
        runTest {
            // Two refreshes answered at once: a read-then-write replace would let both "win".
            val third =
                StoredSession(
                    SessionTokens(accessToken = "access-third", refreshToken = "refresh-third"),
                    other.session,
                )
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
        assertFalse(session.tokens.toString().contains("secret"), session.tokens.toString())
        assertFalse(session.toString().contains("secret"), session.toString())
        assertFalse(session.toString().contains("altarpm"), "no email in logs: $session")
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
