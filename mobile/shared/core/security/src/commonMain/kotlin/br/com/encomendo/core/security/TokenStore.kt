package br.com.encomendo.core.security

import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.byteArrayPreferencesKey
import androidx.datastore.preferences.core.edit
import br.com.encomendo.core.domain.session.Session
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.Json

/**
 * TokenStore is the only place the app keeps the session's tokens
 * (mobile/CLAUDE.md, Auth and offline), with the [Session] they were issued
 * for. It guarantees that:
 *
 * - the session reaches storage only sealed by [cipher] (the Keystore on
 *   Android), as one blob, so the access token, the refresh token and the
 *   permissions always change together;
 * - a session the device can no longer decrypt reads as signed out, and the
 *   user simply logs in again;
 * - [replace] is atomic, so a refresh can never undo a logout or a login.
 *
 * Keep one instance per process: DataStore allows one per file.
 *
 * @param cryptoDispatcher where [cipher] runs; Keystore calls block.
 */
class TokenStore(
    private val dataStore: DataStore<Preferences>,
    private val cipher: SecretCipher,
    private val cryptoDispatcher: CoroutineDispatcher,
) {
    private val lastOpened = MutableStateFlow<Opened?>(null)

    /**
     * Emits the stored session (null when signed out), then every change to it.
     * The tokens stay out of it, so nothing that observes the session can leak them.
     */
    val session: Flow<Session?> = dataStore.data.map { open(it)?.session }.distinctUntilChanged()

    /** Returns the stored session with its tokens, or null when signed out. */
    suspend fun read(): StoredSession? = open(dataStore.data.first())

    /** Stores [session] after a login, replacing any previous session. */
    suspend fun write(session: StoredSession) {
        val sealed = seal(session)
        dataStore.edit { it[SESSION] = sealed }
    }

    /** Signs out on this device. */
    suspend fun clear() {
        dataStore.edit { it.remove(SESSION) }
        forgetOpened()
    }

    /**
     * Replaces the session with [replacement] (null signs out) only while the
     * stored tokens are still [expected]'s. Returns false and changes nothing
     * otherwise. Only the tokens are compared: the refresh token is unique to
     * one session, and a refresh must not lose its rotated tokens because
     * something stored with them changed meanwhile.
     */
    suspend fun replace(
        expected: StoredSession,
        replacement: StoredSession?,
    ): Boolean {
        var replaced = false
        dataStore.edit { preferences ->
            replaced = open(preferences)?.tokens == expected.tokens
            if (!replaced) return@edit
            if (replacement == null) preferences.remove(SESSION) else preferences[SESSION] = seal(replacement)
        }
        if (replaced && replacement == null) forgetOpened()
        return replaced
    }

    /** Drops the ended session's plain tokens from memory; the cache only ever helps the live one. */
    private fun forgetOpened() {
        lastOpened.value = null
    }

    private suspend fun open(preferences: Preferences): StoredSession? = open(preferences[SESSION])

    private suspend fun open(sealed: ByteArray?): StoredSession? {
        if (sealed == null) return null
        // Each API request reads the tokens; decrypt only when the stored blob changed.
        val cached = lastOpened.value?.takeIf { it.sealed.contentEquals(sealed) }
        return if (cached != null) cached.session else decrypt(sealed)
    }

    private suspend fun decrypt(sealed: ByteArray): StoredSession? {
        val session = withContext(cryptoDispatcher) { cipher.open(sealed) }?.let(::decode)
        lastOpened.value = Opened(sealed, session)
        return session
    }

    private suspend fun seal(session: StoredSession): ByteArray {
        val plain = json.encodeToString(session.toRecord()).encodeToByteArray()
        return withContext(cryptoDispatcher) { cipher.seal(plain) }
    }

    private fun decode(plain: ByteArray): StoredSession? =
        try {
            json.decodeFromString<SessionRecord>(plain.decodeToString()).toStoredSession()
        } catch (_: IllegalArgumentException) {
            // SerializationException is an IllegalArgumentException: an old or broken blob means signed out.
            null
        }

    /** The last blob decrypted, so repeated reads skip the Keystore. */
    private class Opened(
        val sealed: ByteArray,
        val session: StoredSession?,
    )

    private companion object {
        val SESSION = byteArrayPreferencesKey("session")
        val json = Json { ignoreUnknownKeys = true }
    }
}
