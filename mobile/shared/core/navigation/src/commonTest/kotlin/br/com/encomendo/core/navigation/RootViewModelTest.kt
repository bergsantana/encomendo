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
import kotlinx.coroutines.flow.first
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
import kotlin.test.assertIs
import kotlin.test.assertNotSame
import kotlin.test.assertTrue

@OptIn(ExperimentalCoroutinesApi::class) // setMain / resetMain / advanceTimeBy
class RootViewModelTest {
    private val main = StandardTestDispatcher()

    @BeforeTest
    fun useTestMain() = Dispatchers.setMain(main)

    @AfterTest
    fun restoreMain() = Dispatchers.resetMain()

    @Test
    fun waitsForTheStoredSessionBeforeChoosingAGraph() =
        runTest(main) {
            val viewModel = rootViewModel(FakeSessionRepository(signedIn = false))

            viewModel.uiState.test {
                assertEquals(RootUiState.Loading, awaitItem(), "no login flash before the session is read")
                assertIs<RootUiState.SignedOut>(awaitItem())
            }
        }

    @Test
    fun opensTheManagementGraphStraightAwayForAStoredSession() =
        runTest(main) {
            val viewModel = rootViewModel(FakeSessionRepository(signedIn = true))

            viewModel.uiState.test {
                skipItems(1) // Loading
                assertIs<RootUiState.SignedIn>(awaitItem())
            }
        }

    @Test
    fun opensTheManagementGraphAfterLoginAndTheAuthGraphAfterLogout() =
        runTest(main) {
            val sessions = FakeSessionRepository(signedIn = false)
            val viewModel = rootViewModel(sessions)

            viewModel.uiState.test {
                skipItems(1) // Loading
                assertIs<RootUiState.SignedOut>(awaitItem())

                sessions.state.value = true
                assertIs<RootUiState.SignedIn>(awaitItem())

                sessions.state.value = false // logout, or a refused refresh
                assertIs<RootUiState.SignedOut>(awaitItem())
            }
        }

    @Test
    fun clearsTheScreensOfAnEndedSession() =
        runTest(main) {
            // The login form must not survive the login, and management screens must not run
            // under the next user's token.
            val sessions = FakeSessionRepository(signedIn = true)
            val viewModel = rootViewModel(sessions)

            viewModel.uiState.test {
                skipItems(1) // Loading
                val signedIn = assertIs<RootUiState.SignedIn>(awaitItem())
                val managementScreen = probeIn(signedIn.screens)

                sessions.state.value = false
                val signedOut = assertIs<RootUiState.SignedOut>(awaitItem())

                assertTrue(managementScreen.cleared)
                assertNotSame(signedIn.screens, signedOut.screens)
            }
        }

    @Test
    fun aQuickLogoutAndLoginStillStartsANewSession() =
        runTest(main) {
            // Out and back in before the screen collects: the state must still change, so the
            // screens never sit on a store that was already cleared.
            val sessions = FakeSessionRepository(signedIn = true)
            val viewModel = rootViewModel(sessions)

            viewModel.uiState.test {
                skipItems(1) // Loading
                val first = assertIs<RootUiState.SignedIn>(awaitItem())
                val oldScreen = probeIn(first.screens)

                sessions.state.value = false
                testScheduler.runCurrent()
                sessions.state.value = true
                testScheduler.runCurrent()

                // Even if the screen only ever saw the last state, it is a different one.
                val second = assertIs<RootUiState.SignedIn>(expectMostRecentItem())
                assertTrue(oldScreen.cleared)
                assertNotSame(first.screens, second.screens)
            }
        }

    @Test
    fun keepsTheScreensWhenTheSameSessionIsObservedAgain() =
        runTest(main) {
            // The app in the background for a while stops and restarts the session flow.
            val viewModel = rootViewModel(FakeSessionRepository(signedIn = true))
            val first = viewModel.uiState.first { it is RootUiState.Session }
            val screen = probeIn(assertIs<RootUiState.SignedIn>(first).screens)
            advanceTimeBy(STOPPED_LONG_ENOUGH_MILLIS)

            viewModel.uiState.test {
                assertEquals(first, awaitItem())
                testScheduler.runCurrent() // the restarted flow re-reads the same session
                expectNoEvents()
            }

            assertFalse(screen.cleared)
        }

    @Test
    fun clearsTheSessionScreensWhenTheAppCloses() =
        runTest(main) {
            val activityStore = ViewModelStore()
            val factory = viewModelFactory { initializer { rootViewModel(FakeSessionRepository()) } }
            val viewModel = ViewModelProvider.create(activityStore, factory).get(RootViewModel::class)
            val signedOut = viewModel.uiState.first { it is RootUiState.Session }
            val screen = probeIn(assertIs<RootUiState.SignedOut>(signedOut).screens)

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

    private fun rootViewModel(sessions: FakeSessionRepository) = RootViewModel(ObserveSignedIn(sessions))

    private fun probeIn(owner: ViewModelStoreOwner): ProbeViewModel =
        ViewModelProvider
            .create(owner, viewModelFactory { initializer { ProbeViewModel() } })
            .get(ProbeViewModel::class)

    private companion object {
        /** Longer than RootViewModel keeps the session flow alive without subscribers. */
        const val STOPPED_LONG_ENOUGH_MILLIS = 6_000L
    }
}
