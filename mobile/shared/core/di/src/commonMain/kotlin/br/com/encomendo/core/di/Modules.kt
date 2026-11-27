package br.com.encomendo.core.di

import br.com.encomendo.core.data.catalog.ApiCatalogRepository
import br.com.encomendo.core.data.catalog.ApiSupplierRepository
import br.com.encomendo.core.data.catalog.CatalogCacheCleaner
import br.com.encomendo.core.data.catalog.CatalogCacheDaos
import br.com.encomendo.core.data.failure.StoreCalls
import br.com.encomendo.core.data.members.ApiInvitationsRepository
import br.com.encomendo.core.data.members.ApiMembersRepository
import br.com.encomendo.core.data.session.ApiAuthRepository
import br.com.encomendo.core.data.session.ApiPasswordResetRepository
import br.com.encomendo.core.data.session.ApiTenancyRepository
import br.com.encomendo.core.data.session.DeviceDataCleaner
import br.com.encomendo.core.data.session.DeviceDataCleaners
import br.com.encomendo.core.data.session.InMemoryStoreSuspensionNotice
import br.com.encomendo.core.data.session.LastStore
import br.com.encomendo.core.data.session.SessionKeeper
import br.com.encomendo.core.data.session.TokenSessionRepository
import br.com.encomendo.core.data.session.createSessionApiClient
import br.com.encomendo.core.data.settings.ApiSettingsRepository
import br.com.encomendo.core.database.EncomendoDatabase
import br.com.encomendo.core.database.buildDatabase
import br.com.encomendo.core.domain.catalog.AttributeRepository
import br.com.encomendo.core.domain.catalog.CatalogRepository
import br.com.encomendo.core.domain.catalog.CategoryRepository
import br.com.encomendo.core.domain.catalog.ItemRepository
import br.com.encomendo.core.domain.catalog.KitRepository
import br.com.encomendo.core.domain.catalog.PhotoRepository
import br.com.encomendo.core.domain.members.InvitationsRepository
import br.com.encomendo.core.domain.members.MembersRepository
import br.com.encomendo.core.domain.session.AuthRepository
import br.com.encomendo.core.domain.session.PasswordResetRepository
import br.com.encomendo.core.domain.session.SessionRepository
import br.com.encomendo.core.domain.session.StoreSuspensionNotice
import br.com.encomendo.core.domain.session.TenancyRepository
import br.com.encomendo.core.domain.settings.SettingsRepository
import br.com.encomendo.core.domain.supplier.SupplierRepository
import br.com.encomendo.core.navigation.ScreenHandover
import br.com.encomendo.core.navigation.navigationModule
import br.com.encomendo.core.network.ApiClientConfig
import br.com.encomendo.core.network.SessionTurns
import br.com.encomendo.core.network.auth.AuthApi
import br.com.encomendo.core.network.catalog.CatalogApis
import br.com.encomendo.core.network.catalog.PhotoStorageApi
import br.com.encomendo.core.network.catalog.SuppliersApi
import br.com.encomendo.core.network.catalog.createStorageClient
import br.com.encomendo.core.network.members.MembersApi
import br.com.encomendo.core.network.settings.SettingsApi
import br.com.encomendo.core.network.tenancy.TenancyApi
import br.com.encomendo.core.security.TokenStore
import br.com.encomendo.feature.auth.di.authModule
import br.com.encomendo.feature.catalogo.di.catalogoModule
import br.com.encomendo.feature.members.di.membersModule
import coil3.ImageLoader
import coil3.PlatformContext
import coil3.network.ktor3.KtorNetworkFetcherFactory
import io.ktor.client.HttpClient
import io.ktor.client.engine.HttpClientEngine
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.IO
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.launch
import org.koin.core.module.Module
import org.koin.core.qualifier.named
import org.koin.dsl.module

/** Names the DataStore that holds the sealed session, apart from any settings DataStore added later. */
val SessionDataStore = named("session-datastore")

/** Names the DataStore of plain hints about this device, such as the store used last. */
val DeviceDataStore = named("device-datastore")

/**
 * Platform bindings: the Room builder, the Keystore cipher, the session and
 * device DataStores, the HTTP engine, the photo picker and, later, the link opener. Each
 * platform provides its own.
 */
expect val platformModule: Module

/** The offline cache and its DAOs. */
val databaseModule: Module =
    module {
        single<EncomendoDatabase> { buildDatabase(builder = get(), queryContext = Dispatchers.IO) }
        single { get<EncomendoDatabase>().syncStateDao() }
        single {
            CatalogCacheDaos(
                pages = get<EncomendoDatabase>().catalogPageDao(),
                details = get<EncomendoDatabase>().catalogDetailDao(),
            )
        }
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
        // One per process: the client's refreshes and every store switch take their turns here.
        single { SessionTurns() }
        // The root says which scope its screens serve; only they get that scope's token (ADR 0018).
        single<ScreenHandover> {
            val turns = get<SessionTurns>()
            ScreenHandover(turns::screensServe)
        }
        single<HttpClient> { createSessionApiClient(engine = get(), config = get(), store = get(), turns = get()) }
        single { AuthApi(get()) }
        single { TenancyApi(get()) }
        single { MembersApi(get()) }
        single { CatalogApis(get()) }
        single { SuppliersApi(get()) }
        single { SettingsApi(get()) }
        // A plain client: the storage host is not the API and must never see the bearer token (ADR 0024).
        single { PhotoStorageApi(createStorageClient(get())) }
    }

/**
 * The image loader of photo thumbnails. Signed photo URLs point at the storage host, not the API, so it loads them
 * through a client with no Auth plugin: the session's bearer token never goes there (ADR 0024, ADR 0026). It is built
 * on the same plain client the photo upload uses.
 */
val imageModule: Module =
    module {
        single<ImageLoader> {
            ImageLoader
                .Builder(get<PlatformContext>())
                .components { add(KtorNetworkFetcherFactory(createStorageClient(get<HttpClientEngine>()))) }
                .build()
        }
        // Sair empties what the device cached for the person leaving: the catalog (the owner's costs) and the
        // pictures (a signed photo URL shows its picture to whoever holds the phone). A store switch keeps both
        // (ADR 0016, ADR 0026).
        single<DeviceDataCleaner> {
            DeviceDataCleaners(
                listOf(CatalogCacheCleaner(get()), CoilDeviceDataCleaner(get<ImageLoader>(), Dispatchers.IO)),
            )
        }
    }

/** Names the scope that lives as long as the app, for work that must outlive the screen that started it. */
val AppScope = named("app-scope")

/** Repository implementations, bound to the interfaces in core:domain. */
val dataModule: Module =
    module {
        single(AppScope) { CoroutineScope(SupervisorJob() + Dispatchers.Default) }
        // Created at start, so it remembers the store of every session from the first one on.
        single(createdAtStart = true) {
            val sessions = get<TokenStore>().session
            LastStore(get(DeviceDataStore)).also { lastStore ->
                get<CoroutineScope>(AppScope).launch { lastStore.follow(sessions) }
            }
        }
        single<SessionRepository> { TokenSessionRepository(tokens = get(), keeper = get()) }
        single<AuthRepository> {
            ApiAuthRepository(
                api = get(),
                store = get(),
                appScope = get(AppScope),
                lastStore = get(),
                deviceData = get(),
            )
        }
        single { SessionKeeper(auth = get(), store = get(), turns = get()) }
        single<TenancyRepository> { ApiTenancyRepository(tenancy = get(), keeper = get()) }
        single<PasswordResetRepository> { ApiPasswordResetRepository(get()) }
        // One per process: the picker reads what the store's own calls raised (ADR 0022).
        single {
            InMemoryStoreSuspensionNotice().also { notice ->
                get<CoroutineScope>(AppScope).launch { notice.follow(get<TokenStore>().session) }
            }
        }
        single<StoreSuspensionNotice> { get<InMemoryStoreSuspensionNotice>() }
        single { StoreCalls(tokens = get(), keeper = get(), notice = get(), renewals = get(AppScope)) }
        single<MembersRepository> { ApiMembersRepository(api = get(), calls = get()) }
        single<InvitationsRepository> { ApiInvitationsRepository(api = get(), calls = get()) }
        single {
            ApiCatalogRepository(
                apis = get(),
                storage = get(),
                calls = get(),
                cache = get(),
                sessions = get(),
            )
        }
        single<CatalogRepository> { get<ApiCatalogRepository>() }
        single<CategoryRepository> { get<ApiCatalogRepository>() }
        single<AttributeRepository> { get<ApiCatalogRepository>() }
        single<ItemRepository> { get<ApiCatalogRepository>() }
        single<KitRepository> { get<ApiCatalogRepository>() }
        single<PhotoRepository> { get<ApiCatalogRepository>() }
        single<SettingsRepository> { ApiSettingsRepository(api = get(), calls = get()) }
        single<SupplierRepository> {
            ApiSupplierRepository(api = get(), calls = get(), cache = get(), sessions = get())
        }
    }

/** Feature modules, in menu order; each joins when it ships. */
val featureModules: List<Module> = listOf(authModule, catalogoModule, membersModule)

/** The modules every platform shares; tests load them with a fake platform module. */
val commonModules: List<Module> =
    listOf(databaseModule, securityModule, networkModule, imageModule, dataModule, navigationModule) + featureModules

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
