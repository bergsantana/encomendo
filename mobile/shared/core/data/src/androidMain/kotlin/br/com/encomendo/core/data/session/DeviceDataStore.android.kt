package br.com.encomendo.core.data.session

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.core.handlers.ReplaceFileCorruptionHandler
import androidx.datastore.preferences.core.PreferenceDataStoreFactory
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.emptyPreferences
import kotlinx.coroutines.CoroutineScope
import okio.Path.Companion.toPath

/** Plain hints about this device, such as [LastStore]'s; never a token (mobile/CLAUDE.md, Auth and offline). */
private const val DEVICE_FILE = "datastore/device.preferences_pb"

/**
 * Returns the DataStore of plain, non-secret hints about this device. Create
 * it once per process. A corrupt file reads as empty: only hints are lost.
 */
fun deviceDataStore(
    context: Context,
    scope: CoroutineScope,
): DataStore<Preferences> =
    PreferenceDataStoreFactory.createWithPath(
        corruptionHandler = ReplaceFileCorruptionHandler { emptyPreferences() },
        scope = scope,
        produceFile = {
            context.applicationContext.filesDir
                .resolve(DEVICE_FILE)
                .absolutePath
                .toPath()
        },
    )
