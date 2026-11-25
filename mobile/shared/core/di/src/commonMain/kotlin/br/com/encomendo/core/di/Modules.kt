package br.com.encomendo.core.di

import br.com.encomendo.core.data.session.ApiAuthRepository
import br.com.encomendo.core.data.session.TokenSessionRepository
import br.com.encomendo.core.data.session.createSessionApiClient
import br.com.encomendo.core.database.EncomendoDatabase
import br.com.encomendo.core.database.buildDatabase
import br.com.encomendo.core.domain.session.AuthRepository
import br.com.encomendo.core.domain.session.SessionRepository
import br.com.encomendo.core.navigation.navigationModule
import br.com.encomendo.core.network.ApiClientConfig
import br.com.encomendo.core.network.auth.AuthApi
import br.com.encomendo.core.security.TokenStore
import br.com.encomendo.feature.auth.di.authModule
import io.ktor.client.HttpClient
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.IO
import kotlinx.coroutines.SupervisorJob
import org.koin.core.module.Module
import org.koin.core.qualifier.named
import org.koin.dsl.module

/** Names the DataStore that holds the sealed session, apart from any settings DataStore added later. */
val SessionDataStore = named("session-datastore")

/**
 * Platform bindings: the Room builder, the Keystore cipher, the session
 * DataStore, the HTTP engine and, later, the camera and link opener. Each
 * platform provides its own.
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

/**
 * The API client: one per process, so its refreshes never run twice at once
 * (docs/api/openapi.yaml, "Sessions"). Needs an [ApiClientConfig] and an
 * `HttpClientEngine`.
 */
val networkModule: Module =
    module {
        single<HttpClient> { createSessionApiClient(engine = get(), config = get(), store = get()) }
        single { AuthApi(get()) }
    }

/** Names the scope that lives as long as the app, for work that must outlive the screen that started it. */
val AppScope = named("app-scope")

/** Repository implementations, bound to the interfaces in core:domain. */
val dataModule: Module =
    module {
        single(AppScope) { CoroutineScope(SupervisorJob() + Dispatchers.Default) }
        single<SessionRepository> { TokenSessionRepository(get()) }
        single<AuthRepository> { ApiAuthRepository(api = get(), store = get(), appScope = get(AppScope)) }
    }

/** Feature modules, in menu order; each joins when it ships. */
val featureModules: List<Module> = listOf(authModule)

/** The modules every platform shares; tests load them with a fake platform module. */
val commonModules: List<Module> =
    listOf(databaseModule, securityModule, networkModule, dataModule, navigationModule) + featureModules

/**
 * Returns every shared module, in the order Koin should load them, talking to
 * the API at [apiBaseUrl] (the app's build type picks it).
 */
fun sharedModules(apiBaseUrl: String): List<Module> =
    listOf(platformModule, apiConfigModule(apiBaseUrl)) + commonModules

/** Binds the [ApiClientConfig] for [apiBaseUrl]; tests use it with their own URL. */
fun apiConfigModule(apiBaseUrl: String): Module =
    module {
        single { ApiClientConfig(baseUrl = apiBaseUrl) }
    }
