package br.com.encomendo.core.data.session

import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.emptyPreferences
import androidx.datastore.preferences.core.stringPreferencesKey
import br.com.encomendo.core.domain.session.TenantSummary
import br.com.encomendo.core.testing.AltaRpm
import br.com.encomendo.core.testing.temporaryPreferencesDataStore
import br.com.encomendo.core.testing.testSession
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.test.runTest
import kotlinx.io.IOException
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

class LastStoreTest {
    private val motoPecas =
        TenantSummary(id = "0190f3c4-0000-7000-8000-000000000002", name = "Moto Peças", slug = "moto-pecas")

    @Test
    fun remembersNothingAtFirst() =
        runTest {
            assertNull(LastStore(temporaryPreferencesDataStore(backgroundScope)).read())
        }

    @Test
    fun followsTheStoreOfEverySessionAndKeepsItAfterLogout() =
        runTest {
            // ADR 0010: login names the store used last on this device, even after Sair. Finite flows,
            // so follow returns once DataStore has written, on its own IO thread.
            val lastStore = LastStore(temporaryPreferencesDataStore(backgroundScope))

            lastStore.follow(flowOf(testSession(tenant = AltaRpm)))
            assertEquals(AltaRpm.id, lastStore.read())

            lastStore.follow(
                flowOf(
                    testSession(tenant = motoPecas),
                    testSession(tenant = null), // signed up, or several stores and none picked
                    null, // Sair
                ),
            )
            assertEquals(motoPecas.id, lastStore.read())
        }

    @Test
    fun aStoredValueThatIsNotAStoreIdReadsAsNone() =
        runTest {
            // A bad tenantId would make every login on this device fail as invalid (as the web's lastStore.ts notes).
            val dataStore = temporaryPreferencesDataStore(backgroundScope)
            dataStore.edit { it[stringPreferencesKey("last_store")] = "not-a-uuid" }

            assertNull(LastStore(dataStore).read())
        }

    @Test
    fun aSessionFileThatCannotBeReadOnlyStopsFollowing() =
        runTest {
            // follow runs in the app's scope, which has no handler: a hint must never crash the app.
            val lastStore = LastStore(temporaryPreferencesDataStore(backgroundScope))

            lastStore.follow(flow { throw IOException("session file unreadable") })

            assertNull(lastStore.read())
        }

    @Test
    fun aDiskThatCannotBeWrittenOnlyLosesTheHint() =
        runTest {
            // The hint saves a tap; failing to save it must never crash the app.
            val lastStore = LastStore(FullDisk)

            lastStore.follow(flowOf(testSession(tenant = AltaRpm)))

            assertNull(lastStore.read())
        }

    private object FullDisk : DataStore<Preferences> {
        override val data: Flow<Preferences> = flowOf(emptyPreferences())

        override suspend fun updateData(transform: suspend (t: Preferences) -> Preferences): Preferences =
            throw IOException("no space left on device")
    }
}
