package br.com.encomendo.core.navigation

import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.ViewModelStore
import androidx.lifecycle.ViewModelStoreOwner
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory
import app.cash.turbine.test
import br.com.encomendo.core.domain.session.ObserveSignedIn
import br.com.encomendo.core.testing.FakeSessionRepository
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import kotlin.test.AfterTest
import kotlin.test.BeforeTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNotSame
import kotlin.test.assertSame
import kotlin.test.assertTrue

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

    @Test
    fun clearsTheScreensOfAnEndedSession() =
        runTest(main) {
            // The login form must not survive the login, and management screens must not run
            // under the next user's token.
            val sessions = FakeSessionRepository(signedIn = true)
            val viewModel = RootViewModel(ObserveSignedIn(sessions))

            viewModel.uiState.test {
                skipItems(1) // Loading
                assertEquals(RootUiState.SignedIn, awaitItem())
                val managementScreen = probeIn(viewModel.sessionViewModels)

                sessions.state.value = false
                assertEquals(RootUiState.SignedOut, awaitItem())

                assertTrue(managementScreen.cleared)
                assertNotSame(managementScreen, probeIn(viewModel.sessionViewModels))
            }
        }

    @Test
    fun keepsTheScreensWhenTheSameSessionIsObservedAgain() =
        runTest(main) {
            // The app in the background for a while stops and restarts the session flow.
            val viewModel = RootViewModel(ObserveSignedIn(FakeSessionRepository(signedIn = true)))
            viewModel.uiState.test {
                skipItems(1)
                awaitItem()
            }
            val screen = probeIn(viewModel.sessionViewModels)
            advanceTimeBy(STOPPED_LONG_ENOUGH_MILLIS)

            viewModel.uiState.test {
                awaitItem()
                expectNoEvents()
            }

            assertFalse(screen.cleared)
            assertSame(screen, probeIn(viewModel.sessionViewModels))
        }

    @Test
    fun clearsTheSessionScreensWhenTheAppCloses() =
        runTest(main) {
            val activityStore = ViewModelStore()
            val factory = viewModelFactory { initializer { RootViewModel(ObserveSignedIn(FakeSessionRepository())) } }
            val viewModel = ViewModelProvider.create(activityStore, factory).get(RootViewModel::class)
            val screen = probeIn(viewModel.sessionViewModels)

            activityStore.clear()

            assertTrue(screen.cleared)
        }

    /** Stands in for a screen's ViewModel and records whether it was cleared. */
    private class ProbeViewModel : ViewModel() {
        var cleared = false
            private set

        override fun onCleared() {
            cleared = true
        }
    }

    private fun probeIn(owner: ViewModelStoreOwner): ProbeViewModel =
        ViewModelProvider
            .create(owner, viewModelFactory { initializer { ProbeViewModel() } })
            .get(ProbeViewModel::class)

    private companion object {
        /** Longer than RootViewModel keeps the session flow alive without subscribers. */
        const val STOPPED_LONG_ENOUGH_MILLIS = 6_000L
    }
}
