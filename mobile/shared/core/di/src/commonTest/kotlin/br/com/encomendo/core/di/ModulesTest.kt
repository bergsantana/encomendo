package br.com.encomendo.core.di

import br.com.encomendo.core.domain.session.SessionRepository
import br.com.encomendo.core.navigation.RootViewModel
import br.com.encomendo.core.security.SecretCipher
import br.com.encomendo.core.security.TokenStore
import br.com.encomendo.core.testing.FakeSecretCipher
import br.com.encomendo.core.testing.temporaryPreferencesDataStore
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
import kotlin.test.assertSame

@OptIn(ExperimentalCoroutinesApi::class) // setMain / resetMain: RootViewModel uses viewModelScope
class ModulesTest {
    @BeforeTest
    fun useTestMain() = Dispatchers.setMain(StandardTestDispatcher())

    @AfterTest
    fun restoreMain() = Dispatchers.resetMain()

    @Test
    fun resolvesTheSessionGraphFromTheCommonModules() =
        runTest {
            val fakePlatform =
                module {
                    single<SecretCipher> { FakeSecretCipher() }
                    single(SessionDataStore) { temporaryPreferencesDataStore(backgroundScope) }
                }
            val koin = koinApplication { modules(listOf(fakePlatform) + commonModules) }.koin

            koin.get<RootViewModel>()
            koin.get<SessionRepository>()
            assertSame(koin.get<TokenStore>(), koin.get<TokenStore>(), "one TokenStore per process")
        }
}
