package br.com.encomendo.core.navigation

import app.cash.turbine.test
import br.com.encomendo.core.domain.session.ObserveSignedIn
import br.com.encomendo.core.testing.FakeSessionRepository
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import kotlin.test.AfterTest
import kotlin.test.BeforeTest
import kotlin.test.Test
import kotlin.test.assertEquals

@OptIn(ExperimentalCoroutinesApi::class) // setMain / resetMain
class RootViewModelTest {
    private val main = StandardTestDispatcher()

    @BeforeTest
    fun useTestMain() = Dispatchers.setMain(main)

    @AfterTest
    fun restoreMain() = Dispatchers.resetMain()

    @Test
    fun waitsForTheStoredSessionBeforeChoosingAGraph() =
        runTest(main) {
            val viewModel = RootViewModel(ObserveSignedIn(FakeSessionRepository(signedIn = false)))

            viewModel.uiState.test {
                assertEquals(RootUiState.Loading, awaitItem(), "no login flash before the session is read")
                assertEquals(RootUiState.SignedOut, awaitItem())
            }
        }

    @Test
    fun opensTheManagementGraphAfterLoginAndTheAuthGraphAfterLogout() =
        runTest(main) {
            val sessions = FakeSessionRepository(signedIn = false)
            val viewModel = RootViewModel(ObserveSignedIn(sessions))

            viewModel.uiState.test {
                skipItems(1) // Loading
                assertEquals(RootUiState.SignedOut, awaitItem())

                sessions.state.value = true
                assertEquals(RootUiState.SignedIn, awaitItem())

                sessions.state.value = false // logout, or a refused refresh
                assertEquals(RootUiState.SignedOut, awaitItem())
            }
        }

    @Test
    fun opensTheManagementGraphStraightAwayForAStoredSession() =
        runTest(main) {
            val viewModel = RootViewModel(ObserveSignedIn(FakeSessionRepository(signedIn = true)))

            viewModel.uiState.test {
                skipItems(1) // Loading
                assertEquals(RootUiState.SignedIn, awaitItem())
            }
        }
}
