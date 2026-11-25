package br.com.encomendo.core.security

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.PreferenceDataStoreFactory
import androidx.datastore.preferences.core.Preferences
import kotlinx.coroutines.CoroutineScope
import okio.Path.Companion.toPath

/** The session file in the app's private storage; it only ever holds sealed bytes. */
private const val SESSION_FILE = "datastore/session.preferences_pb"

/** Returns the DataStore that [TokenStore] keeps the sealed session in. Create it once per process. */
fun sessionDataStore(
    context: Context,
    scope: CoroutineScope,
): DataStore<Preferences> =
    PreferenceDataStoreFactory.createWithPath(
        scope = scope,
        produceFile = {
            context.applicationContext.filesDir
                .resolve(SESSION_FILE)
                .absolutePath
                .toPath()
        },
    )
