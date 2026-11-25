package br.com.encomendo.core.di

import br.com.encomendo.core.data.session.deviceDataStore
import br.com.encomendo.core.database.databaseBuilder
import br.com.encomendo.core.network.okHttpEngine
import br.com.encomendo.core.security.KeystoreCipher
import br.com.encomendo.core.security.SecretCipher
import br.com.encomendo.core.security.sessionDataStore
import io.ktor.client.engine.HttpClientEngine
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import org.koin.android.ext.koin.androidContext
import org.koin.core.module.Module
import org.koin.dsl.module

/** Android bindings; needs androidContext() from startKoin in the Application. */
actual val platformModule: Module =
    module {
        single { databaseBuilder(androidContext()) }
        single<SecretCipher> { KeystoreCipher() }
        single(SessionDataStore) {
            sessionDataStore(androidContext(), scope = CoroutineScope(Dispatchers.IO + SupervisorJob()))
        }
        single(DeviceDataStore) {
            deviceDataStore(androidContext(), scope = CoroutineScope(Dispatchers.IO + SupervisorJob()))
        }
        single<HttpClientEngine> { okHttpEngine() }
    }
