package br.com.encomendo.core.di

import br.com.encomendo.core.domain.session.AuthRepository
import br.com.encomendo.core.domain.session.SessionRepository
import br.com.encomendo.core.navigation.RootViewModel
import br.com.encomendo.core.network.ApiClientConfig
import br.com.encomendo.core.security.SecretCipher
import br.com.encomendo.core.security.TokenStore
import br.com.encomendo.core.testing.FakeSecretCipher
import br.com.encomendo.core.testing.temporaryPreferencesDataStore
import br.com.encomendo.feature.auth.AccountViewModel
import br.com.encomendo.feature.auth.LoginViewModel
import io.ktor.client.HttpClient
import io.ktor.client.engine.HttpClientEngine
import io.ktor.client.engine.mock.MockEngine
import io.ktor.client.engine.mock.respondOk
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.koin.dsl.koinApplication
import org.koin.dsl.module
import kotlin.test.AfterTest
import kotlin.test.BeforeTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertSame

@OptIn(ExperimentalCoroutinesApi::class) // setMain / resetMain: the ViewModels use viewModelScope
class ModulesTest {
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
                    single(SessionDataStore) { temporaryPreferencesDataStore(backgroundScope) }
                    single<HttpClientEngine> { MockEngine { respondOk() } }
                }
            val modules = listOf(fakePlatform, apiConfigModule("https://api.test")) + commonModules
            val koin = koinApplication { modules(modules) }.koin

            koin.get<RootViewModel>()
            koin.get<LoginViewModel>()
            koin.get<AccountViewModel>()
            koin.get<SessionRepository>()
            koin.get<AuthRepository>()
            assertSame(koin.get<TokenStore>(), koin.get<TokenStore>(), "one TokenStore per process")
            assertSame(koin.get<HttpClient>(), koin.get<HttpClient>(), "one API client, so refreshes never race")
            assertSame(koin.get<CoroutineScope>(AppScope), koin.get<CoroutineScope>(AppScope), "one app scope")
            assertEquals("https://api.test", koin.get<ApiClientConfig>().baseUrl)
        }
}
