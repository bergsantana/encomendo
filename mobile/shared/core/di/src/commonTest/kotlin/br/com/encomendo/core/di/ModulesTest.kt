package br.com.encomendo.core.di

import androidx.room3.Room
import androidx.room3.RoomDatabase
import br.com.encomendo.core.data.session.DeviceDataCleaner
import br.com.encomendo.core.database.EncomendoDatabase
import br.com.encomendo.core.domain.catalog.AttributeRepository
import br.com.encomendo.core.domain.catalog.CatalogRepository
import br.com.encomendo.core.domain.catalog.CategoryRepository
import br.com.encomendo.core.domain.catalog.ItemRepository
import br.com.encomendo.core.domain.catalog.KitRepository
import br.com.encomendo.core.domain.catalog.PhotoRepository
import br.com.encomendo.core.domain.session.AuthRepository
import br.com.encomendo.core.domain.session.SessionRepository
import br.com.encomendo.core.domain.session.TenancyRepository
import br.com.encomendo.core.domain.session.scope
import br.com.encomendo.core.domain.settings.SettingsRepository
import br.com.encomendo.core.domain.supplier.SupplierRepository
import br.com.encomendo.core.navigation.RootViewModel
import br.com.encomendo.core.navigation.ScreenHandover
import br.com.encomendo.core.network.ApiClientConfig
import br.com.encomendo.core.network.SessionTurns
import br.com.encomendo.core.platform.PhotoPicker
import br.com.encomendo.core.security.SecretCipher
import br.com.encomendo.core.security.SessionTokens
import br.com.encomendo.core.security.StoredSession
import br.com.encomendo.core.security.TokenStore
import br.com.encomendo.core.testing.FakePhotoPicker
import br.com.encomendo.core.testing.FakeSecretCipher
import br.com.encomendo.core.testing.temporaryPreferencesDataStore
import br.com.encomendo.core.testing.testSession
import br.com.encomendo.feature.auth.AccountViewModel
import br.com.encomendo.feature.auth.LoginViewModel
import br.com.encomendo.feature.auth.StorePickerViewModel
import br.com.encomendo.feature.catalogo.AttributesViewModel
import br.com.encomendo.feature.catalogo.CategoriesViewModel
import br.com.encomendo.feature.catalogo.ItemFormViewModel
import br.com.encomendo.feature.catalogo.ItemsViewModel
import br.com.encomendo.feature.members.MembersViewModel
import coil3.ImageLoader
import coil3.PlatformContext
import io.ktor.client.HttpClient
import io.ktor.client.engine.HttpClientEngine
import io.ktor.client.engine.mock.MockEngine
import io.ktor.client.engine.mock.respondOk
import io.ktor.client.request.get
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import kotlinx.coroutines.withContext
import kotlinx.coroutines.withTimeout
import kotlinx.coroutines.withTimeoutOrNull
import org.koin.core.parameter.parametersOf
import org.koin.dsl.koinApplication
import org.koin.dsl.module
import kotlin.test.AfterTest
import kotlin.test.BeforeTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull
import kotlin.test.assertSame

@OptIn(ExperimentalCoroutinesApi::class) // setMain / resetMain: the ViewModels use viewModelScope
class ModulesTest {
    private companion object {
        /** Real time: the engine answers on its own thread. */
        const val RELEASE_MILLIS = 5_000L

        /** Real time a miswired client would need to answer; it doesn't wait at all. */
        const val WAIT_MILLIS = 300L
    }

    @BeforeTest
    fun useTestMain() = Dispatchers.setMain(StandardTestDispatcher())

    @AfterTest
    fun restoreMain() = Dispatchers.resetMain()

    @Test
    fun resolvesTheAppGraphFromTheCommonModules() =
        runTest {
            val fakePlatform =
                module {
                    single<SecretCipher> { FakeSecretCipher() }
                    single<PhotoPicker> { FakePhotoPicker() }
                    single<PlatformContext> { testPlatformContext() }
                    single(SessionDataStore) { temporaryPreferencesDataStore(backgroundScope) }
                    single(DeviceDataStore) { temporaryPreferencesDataStore(backgroundScope) }
                    single<HttpClientEngine> { MockEngine { respondOk() } }
                    // The cache DAOs are only built here, never queried, so no SQLite is opened.
                    single<RoomDatabase.Builder<EncomendoDatabase>> {
                        Room.inMemoryDatabaseBuilder<EncomendoDatabase>()
                    }
                }
            // The test's own scope as the app's, so nothing started at launch outlives the test.
            val testAppScope = module { single(AppScope) { backgroundScope } }
            val modules = listOf(fakePlatform, apiConfigModule("https://api.test")) + commonModules + testAppScope
            val koin = koinApplication { modules(modules) }.koin

            koin.get<RootViewModel>()
            koin.get<LoginViewModel>()
            koin.get<AccountViewModel>()
            koin.get<StorePickerViewModel>()
            koin.get<MembersViewModel>()
            koin.get<ItemsViewModel>()
            koin.get<CategoriesViewModel>()
            koin.get<AttributesViewModel>()
            koin.get<ItemFormViewModel> { parametersOf(null) }
            koin.get<SettingsRepository>()
            // The photo thumbnails' loader: built over the storage client, which has no Auth plugin.
            koin.get<ImageLoader>()
            koin.get<DeviceDataCleaner>().clear() // Sair empties the loader's caches; an empty cache is fine
            koin.get<CatalogRepository>()
            koin.get<SupplierRepository>()
            koin.get<CategoryRepository>()
            koin.get<AttributeRepository>()
            koin.get<ItemRepository>()
            koin.get<KitRepository>()
            koin.get<PhotoRepository>()
            koin.get<TenancyRepository>()
            assertSame(
                koin.get<SessionTurns>(),
                koin.get<SessionTurns>(),
                "one turn for every call that spends the refresh token",
            )
            koin.get<SessionRepository>()
            koin.get<AuthRepository>()
            assertSame(koin.get<TokenStore>(), koin.get<TokenStore>(), "one TokenStore per process")
            assertSame(koin.get<HttpClient>(), koin.get<HttpClient>(), "one API client, so refreshes never race")
            assertSame(koin.get<CoroutineScope>(AppScope), koin.get<CoroutineScope>(AppScope), "one app scope")
            assertEquals("https://api.test", koin.get<ApiClientConfig>().baseUrl)

            // The root's handover must reach the client's own turns: wired to others, the client would hand
            // any token to any screens. So first the request waits for screens of the stored scope.
            val stored = StoredSession(SessionTokens("access", "refresh"), testSession())
            koin.get<TokenStore>().write(stored)
            val handover = koin.get<ScreenHandover>()
            val client = koin.get<HttpClient>()
            handover.screensReplaced("someone-else@")
            val early = withContext(Dispatchers.Default) { withTimeoutOrNull(WAIT_MILLIS) { client.get("/readyz") } }
            assertNull(early, "waits for screens of the stored scope")
            handover.screensReplaced(stored.session.scope)
            withContext(Dispatchers.Default) { withTimeout(RELEASE_MILLIS) { client.get("/readyz") } }
        }
}
