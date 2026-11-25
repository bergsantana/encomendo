package br.com.encomendo.core.di

import br.com.encomendo.core.data.session.TokenSessionRepository
import br.com.encomendo.core.database.EncomendoDatabase
import br.com.encomendo.core.database.buildDatabase
import br.com.encomendo.core.domain.session.SessionRepository
import br.com.encomendo.core.navigation.navigationModule
import br.com.encomendo.core.security.TokenStore
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.IO
import org.koin.core.module.Module
import org.koin.core.qualifier.named
import org.koin.dsl.module

/** Names the DataStore that holds the sealed session, apart from any settings DataStore added later. */
val SessionDataStore = named("session-datastore")

/**
 * Platform bindings: the Room builder, the Keystore cipher and the session
 * DataStore and, later, the camera and link opener. Each platform provides its own.
 */
expect val platformModule: Module

/** The offline cache and its DAOs. */
val databaseModule: Module =
    module {
        single<EncomendoDatabase> { buildDatabase(builder = get(), queryContext = Dispatchers.IO) }
        single { get<EncomendoDatabase>().syncStateDao() }
    }

/** The token store: one per process, because DataStore allows one instance per file. */
val securityModule: Module =
    module {
        single { TokenStore(dataStore = get(SessionDataStore), cipher = get(), cryptoDispatcher = Dispatchers.IO) }
    }

/** Repository implementations, bound to the interfaces in core:domain. */
val dataModule: Module =
    module {
        single<SessionRepository> { TokenSessionRepository(get()) }
    }

/** The modules every platform shares; tests load them with a fake platform module. */
val commonModules: List<Module> = listOf(databaseModule, securityModule, dataModule, navigationModule)

/** Every shared module, in the order Koin should load them. Feature modules join as they ship. */
val sharedModules: List<Module> = listOf(platformModule) + commonModules
