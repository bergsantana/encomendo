package br.com.encomendo.core.security

import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.byteArrayPreferencesKey
import androidx.datastore.preferences.core.edit
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.withContext
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json

/**
 * TokenStore is the only place the app keeps the session's tokens
 * (mobile/CLAUDE.md, Auth and offline). It guarantees that:
 *
 * - tokens reach storage only sealed by [cipher] (the Keystore on Android),
 *   as one blob, so the access and refresh tokens always change together;
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

    /** Emits whether a readable session is stored, then every change between signed in and out. */
    val signedIn: Flow<Boolean> = dataStore.data.map { open(it) != null }.distinctUntilChanged()

    /** Returns the stored tokens, or null when signed out. */
    suspend fun read(): SessionTokens? = open(dataStore.data.first())

    /** Stores [tokens] after a login, replacing any previous session. */
    suspend fun write(tokens: SessionTokens) {
        val sealed = seal(tokens)
        dataStore.edit { it[SESSION] = sealed }
    }

    /** Signs out on this device. */
    suspend fun clear() {
        dataStore.edit { it.remove(SESSION) }
        forgetOpened()
    }

    /**
     * Replaces the session with [replacement] (null signs out) only while the
     * stored session is still [expected]. Returns false and changes nothing
     * otherwise.
     */
    suspend fun replace(
        expected: SessionTokens,
        replacement: SessionTokens?,
    ): Boolean {
        var replaced = false
        dataStore.edit { preferences ->
            replaced = open(preferences) == expected
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

    private suspend fun open(preferences: Preferences): SessionTokens? = open(preferences[SESSION])

    private suspend fun open(sealed: ByteArray?): SessionTokens? {
        if (sealed == null) return null
        // Each API request reads the tokens; decrypt only when the stored blob changed.
        val cached = lastOpened.value?.takeIf { it.sealed.contentEquals(sealed) }
        return if (cached != null) cached.tokens else decrypt(sealed)
    }

    private suspend fun decrypt(sealed: ByteArray): SessionTokens? {
        val tokens = withContext(cryptoDispatcher) { cipher.open(sealed) }?.let(::decode)
        lastOpened.value = Opened(sealed, tokens)
        return tokens
    }

    private suspend fun seal(tokens: SessionTokens): ByteArray {
        val plain = json.encodeToString(StoredSession(tokens.accessToken, tokens.refreshToken)).encodeToByteArray()
        return withContext(cryptoDispatcher) { cipher.seal(plain) }
    }

    private fun decode(plain: ByteArray): SessionTokens? =
        try {
            val stored = json.decodeFromString<StoredSession>(plain.decodeToString())
            SessionTokens(accessToken = stored.access, refreshToken = stored.refresh)
        } catch (_: IllegalArgumentException) {
            // SerializationException is an IllegalArgumentException: an old or broken blob means signed out.
            null
        }

    /** The last blob decrypted, so repeated reads skip the Keystore. */
    private class Opened(
        val sealed: ByteArray,
        val tokens: SessionTokens?,
    )

    @Serializable
    private data class StoredSession(
        val access: String,
        val refresh: String,
    )

    private companion object {
        val SESSION = byteArrayPreferencesKey("session")
        val json = Json { ignoreUnknownKeys = true }
    }
}
