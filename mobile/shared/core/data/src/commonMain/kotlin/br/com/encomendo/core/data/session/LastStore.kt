package br.com.encomendo.core.data.session

import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import br.com.encomendo.core.domain.session.Session
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.catch
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.mapNotNull
import kotlinx.io.IOException

/**
 * LastStore remembers the store this device used last, so the next login
 * opens it (`LoginRequest.tenantId`, ADR 0010). It outlives logout. A store
 * id is not a secret, so it lives in plain [dataStore], never in `TokenStore`.
 */
class LastStore(
    private val dataStore: DataStore<Preferences>,
) {
    /** Returns the id of the store used last, or null when there is none or it can't be read. */
    suspend fun read(): String? =
        try {
            dataStore.data.first()[LAST_STORE]?.takeIf(UUID::matches)
        } catch (_: IOException) {
            null
        }

    /**
     * Remembers the store of every session [sessions] emits: a login, a
     * refresh, a switch. A session without a store changes nothing. Runs until
     * cancelled; start it once, in the app's scope.
     */
    suspend fun follow(sessions: Flow<Session?>) {
        sessions
            .mapNotNull { it?.tenant?.id }
            .distinctUntilChanged()
            // An unreadable session file stops the hint, never the app.
            .catch { if (it !is IOException) throw it }
            .collect(::remember)
    }

    private suspend fun remember(tenantId: String) {
        try {
            dataStore.edit { it[LAST_STORE] = tenantId }
        } catch (_: IOException) {
            // Only a hint is lost: the next login picks the store again.
        }
    }

    private companion object {
        val LAST_STORE = stringPreferencesKey("last_store")

        /** Store ids are UUIDs; anything else would make every login fail as `invalid`. */
        val UUID = Regex("^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$")
    }
}
