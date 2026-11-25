package br.com.encomendo.core.testing

import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.PreferenceDataStoreFactory
import androidx.datastore.preferences.core.Preferences
import kotlinx.coroutines.CoroutineScope
import okio.FileSystem
import kotlin.random.Random

/**
 * Returns a preferences DataStore in a fresh temporary file, owned by [scope]
 * (use `backgroundScope` in `runTest`), so each test starts empty.
 */
fun temporaryPreferencesDataStore(scope: CoroutineScope): DataStore<Preferences> {
    val file = FileSystem.SYSTEM_TEMPORARY_DIRECTORY / "encomendo-test-${Random.nextLong().toULong()}.preferences_pb"
    return PreferenceDataStoreFactory.createWithPath(scope = scope, produceFile = { file })
}
