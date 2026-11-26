package br.com.encomendo.core.navigation

import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.ViewModelStore
import androidx.lifecycle.ViewModelStoreOwner
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory
import app.cash.turbine.test
import br.com.encomendo.core.domain.session.GrantedPermissions
import br.com.encomendo.core.domain.session.ObserveSession
import br.com.encomendo.core.domain.session.Permission
import br.com.encomendo.core.domain.session.RenewSession
import br.com.encomendo.core.domain.session.TenantSummary
import br.com.encomendo.core.domain.session.scope
import br.com.encomendo.core.testing.FakeSessionRepository
import br.com.encomendo.core.testing.testSession
import kotlinx.coroutines.CompletableDeferred
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
import kotlin.test.assertSame
import kotlin.test.assertTrue

@OptIn(ExperimentalCoroutinesApi::class) // setMain / resetMain / advanceTimeBy
class RootViewModelTest {
    private val main = StandardTestDispatcher()
    private val motoPecas =
        TenantSummary(id = "0190f3c4-0000-7000-8000-000000000002", name = "Moto Peças", slug = "moto-pecas")

    @BeforeTest
    fun useTestMain() = Dispatchers.setMain(main)

    @AfterTest
    fun restoreMain() = Dispatchers.resetMain()

    @Test
    fun renewsTheSessionOnceWhenTheAppStarts() =
        runTest(main) {
            // ADR 0010: each launch brings the role and store the server has now, without waiting for a
            // 401. The root survives rotation; a new session, or coming back from the background, renews
            // nothing more.
            val sessions = FakeSessionRepository(testSession())
            val viewModel = rootViewModel(sessions)

            viewModel.uiState.test {
                skipItems(1) // Loading
                awaitItem()
                sessions.state.value = testSession(tenant = motoPecas)
                awaitItem()
            }
            advanceTimeBy(STOPPED_LONG_ENOUGH_MILLIS) // the session flow stops, as in the background
            viewModel.uiState.test {
                awaitItem()
                cancelAndIgnoreRemainingEvents()
            }

            assertEquals(1, sessions.renewals)
        }

    @Test
    fun theStoredSessionOpensWithoutWaitingForTheRenewal() =
        runTest(main) {
            // Offline cold start (ADR 0012, ADR 0015): the app opens with what it had while the renewal waits.
            val sessions = FakeSessionRepository(testSession()).apply { renewalGate = CompletableDeferred() }
            val viewModel = rootViewModel(sessions)

            viewModel.uiState.test {
                skipItems(1) // Loading
                assertIs<RootUiState.SignedIn>(awaitItem())
            }
            assertEquals(1, sessions.renewals)
        }

    @Test
    fun waitsForTheStoredSessionBeforeChoosingAGraph() =
        runTest(main) {
            val viewModel = rootViewModel(FakeSessionRepository(session = null))

            viewModel.uiState.test {
                assertEquals(RootUiState.Loading, awaitItem(), "no login flash before the session is read")
                assertIs<RootUiState.SignedOut>(awaitItem())
            }
        }

    @Test
    fun opensTheManagementGraphStraightAwayForAStoredSession() =
        runTest(main) {
            val viewModel = rootViewModel(FakeSessionRepository(testSession()))

            viewModel.uiState.test {
                skipItems(1) // Loading
                assertIs<RootUiState.SignedIn>(awaitItem())
            }
        }

    @Test
    fun opensTheManagementGraphAfterLoginAndTheAuthGraphAfterLogout() =
        runTest(main) {
            val sessions = FakeSessionRepository(session = null)
            val viewModel = rootViewModel(sessions)

            viewModel.uiState.test {
                skipItems(1) // Loading
                assertIs<RootUiState.SignedOut>(awaitItem())

                sessions.state.value = testSession()
                assertIs<RootUiState.SignedIn>(awaitItem())

                sessions.state.value = null // logout, or a refused refresh
                assertIs<RootUiState.SignedOut>(awaitItem())
            }
        }

    @Test
    fun clearsTheScreensOfAnEndedSession() =
        runTest(main) {
            // The login form must not survive the login, and management screens must not run
            // under the next user's token.
            val sessions = FakeSessionRepository(testSession())
            val viewModel = rootViewModel(sessions)

            viewModel.uiState.test {
                skipItems(1) // Loading
                val signedIn = assertIs<RootUiState.SignedIn>(awaitItem())
                val managementScreen = probeIn(signedIn.screens)

                sessions.state.value = null
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
            val sessions = FakeSessionRepository(testSession())
            val viewModel = rootViewModel(sessions)

            viewModel.uiState.test {
                skipItems(1) // Loading
                val first = assertIs<RootUiState.SignedIn>(awaitItem())
                val oldScreen = probeIn(first.screens)

                sessions.state.value = null
                testScheduler.runCurrent()
                sessions.state.value = testSession()
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
            val viewModel = rootViewModel(FakeSessionRepository(testSession()))
            val first = viewModel.uiState.first { it is RootUiState.Ready }
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
            val signedOut = viewModel.uiState.first { it is RootUiState.Ready }
            val screen = probeIn(assertIs<RootUiState.SignedOut>(signedOut).screens)

            activityStore.clear()

            assertTrue(screen.cleared)
        }

    @Test
    fun givesTheManagementGraphTheSessionsPermissions() =
        runTest(main) {
            val viewModel = rootViewModel(FakeSessionRepository(testSession(permissions = setOf("order:write"))))

            val signedIn = assertIs<RootUiState.SignedIn>(viewModel.uiState.first { it is RootUiState.Ready })

            assertEquals(GrantedPermissions(listOf(Permission("order:write"))), signedIn.permissions)
        }

    @Test
    fun saysWhichScopeTheNewScreensServeOnceTheOldOnesAreGone() =
        runTest(main) {
            // Only then may the new store's token go out (ADR 0018): no screen of the old store is left.
            val sessions = FakeSessionRepository(testSession())
            val reported = mutableListOf<String?>()
            var oldScreen: ProbeViewModel? = null
            var clearedWhenReported: Boolean? = null
            val viewModel =
                rootViewModel(sessions) { scope ->
                    reported += scope
                    clearedWhenReported = oldScreen?.cleared
                }

            viewModel.uiState.test {
                skipItems(1) // Loading
                oldScreen = probeIn(assertIs<RootUiState.SignedIn>(awaitItem()).screens)
                val moved = testSession(tenant = motoPecas)

                sessions.state.value = moved
                awaitItem()

                assertEquals(listOf<String?>(testSession().scope, moved.scope), reported)
                assertEquals(true, clearedWhenReported, "cleared before the new scope is served")
            }
        }

    @Test
    fun reportsNoScopeOnceTheRootItselfIsGone() =
        runTest(main) {
            // No screens are left; anything still running outside them must not keep a store's scope.
            val reported = mutableListOf<String?>()
            val owner = ViewModelStore()
            val viewModel =
                ViewModelProvider
                    .create(
                        object : ViewModelStoreOwner {
                            override val viewModelStore = owner
                        },
                        viewModelFactory {
                            initializer {
                                rootViewModel(FakeSessionRepository(testSession())) {
                                    reported +=
                                        it
                                }
                            }
                        },
                    ).get(RootViewModel::class)

            viewModel.uiState.test {
                skipItems(1) // Loading
                awaitItem()
                owner.clear()
                cancelAndIgnoreRemainingEvents()
            }

            assertEquals(listOf(testSession().scope, null), reported)
        }

    @Test
    fun reportsNoScopeWhileSignedOut() =
        runTest(main) {
            val reported = mutableListOf<String?>()
            val viewModel = rootViewModel(FakeSessionRepository(session = null)) { reported += it }

            viewModel.uiState.test {
                skipItems(1) // Loading
                awaitItem()
                assertEquals(listOf<String?>(null), reported)
            }
        }

    @Test
    fun aRefreshOfTheSameStoreHandsNothingOver() =
        runTest(main) {
            val sessions = FakeSessionRepository(testSession(permissions = setOf("order:read")))
            val reported = mutableListOf<String?>()
            val viewModel = rootViewModel(sessions) { reported += it }

            viewModel.uiState.test {
                skipItems(1) // Loading
                awaitItem()
                sessions.state.value = testSession(permissions = setOf("order:read", "order:write"))
                awaitItem()

                assertEquals(1, reported.size)
            }
        }

    @Test
    fun aRefreshWithNewPermissionsKeepsTheScreens() =
        runTest(main) {
            // A role change reaches the app with the next refresh (ADR 0010); the open screens stay.
            val sessions = FakeSessionRepository(testSession(permissions = setOf("order:read")))
            val viewModel = rootViewModel(sessions)

            viewModel.uiState.test {
                skipItems(1) // Loading
                val before = assertIs<RootUiState.SignedIn>(awaitItem())
                val screen = probeIn(before.screens)

                sessions.state.value = testSession(permissions = setOf("order:read", "order:write"))
                val after = assertIs<RootUiState.SignedIn>(awaitItem())

                assertEquals(
                    GrantedPermissions(listOf(Permission("order:read"), Permission("order:write"))),
                    after.permissions,
                )
                assertSame(before.screens, after.screens)
                assertFalse(screen.cleared)
            }
        }

    @Test
    fun switchingStoresClearsTheScreens() =
        runTest(main) {
            // A screen loaded for one store must not keep showing it, or keep working on it,
            // once the session moved to another (ADR 0012, Consequences).
            val other =
                TenantSummary(id = "0190f3c4-0000-7000-8000-000000000002", name = "Moto Peças", slug = "moto-pecas")
            val sessions = FakeSessionRepository(testSession())
            val viewModel = rootViewModel(sessions)

            viewModel.uiState.test {
                skipItems(1) // Loading
                val first = assertIs<RootUiState.SignedIn>(awaitItem())
                val screen = probeIn(first.screens)

                sessions.state.value = testSession(tenant = other)
                val second = assertIs<RootUiState.SignedIn>(awaitItem())

                assertTrue(screen.cleared)
                assertNotSame(first.screens, second.screens)
            }
        }

    @Test
    fun anotherUserInTheSameStoreGetsNewScreens() =
        runTest(main) {
            // A new login over the old one, with no signed-out state in between.
            val sessions = FakeSessionRepository(testSession())
            val viewModel = rootViewModel(sessions)

            viewModel.uiState.test {
                skipItems(1) // Loading
                val first = assertIs<RootUiState.SignedIn>(awaitItem())
                val screen = probeIn(first.screens)

                sessions.state.value = testSession(userId = "0190f3c4-0000-7000-8000-0000000000bb")
                val second = assertIs<RootUiState.SignedIn>(awaitItem())

                assertTrue(screen.cleared)
                assertNotSame(first.screens, second.screens)
            }
        }

    @Test
    fun aRefreshThatDropsTheStoreClearsTheScreensAndOpensThePicker() =
        runTest(main) {
            // Removed from the store: the refresh answers with no store selected (docs/api/openapi.yaml).
            val sessions = FakeSessionRepository(testSession(permissions = setOf("order:write")))
            val viewModel = rootViewModel(sessions)

            viewModel.uiState.test {
                skipItems(1) // Loading
                val first = assertIs<RootUiState.SignedIn>(awaitItem())
                val screen = probeIn(first.screens)

                sessions.state.value = testSession(tenant = null)
                assertIs<RootUiState.ChoosingStore>(awaitItem())

                assertTrue(screen.cleared)
            }
        }

    @Test
    fun aSessionWithoutAStoreOpensThePickerNotTheManagementGraph() =
        runTest(main) {
            // Right after sign-up, or with several stores and none picked: store routes would answer
            // 403 tenant_required (ADR 0010).
            val viewModel = rootViewModel(FakeSessionRepository(testSession(tenant = null)))

            viewModel.uiState.test {
                skipItems(1) // Loading
                assertIs<RootUiState.ChoosingStore>(awaitItem())
            }
        }

    @Test
    fun choosingAStoreClearsThePickersScreens() =
        runTest(main) {
            val sessions = FakeSessionRepository(testSession(tenant = null))
            val viewModel = rootViewModel(sessions)

            viewModel.uiState.test {
                skipItems(1) // Loading
                val picking = assertIs<RootUiState.ChoosingStore>(awaitItem())
                val picker = probeIn(picking.screens)

                sessions.state.value = testSession()
                assertIs<RootUiState.SignedIn>(awaitItem())

                assertTrue(picker.cleared)
            }
        }

    /** Stands in for a screen's ViewModel and records whether it was cleared. */
    private class ProbeViewModel : ViewModel() {
        var cleared = false
            private set

        override fun onCleared() {
            cleared = true
        }
    }

    private fun rootViewModel(
        sessions: FakeSessionRepository,
        handover: ScreenHandover = ScreenHandover { },
    ) = RootViewModel(ObserveSession(sessions), RenewSession(sessions), handover)

    private fun probeIn(owner: ViewModelStoreOwner): ProbeViewModel =
        ViewModelProvider
            .create(owner, viewModelFactory { initializer { ProbeViewModel() } })
            .get(ProbeViewModel::class)

    private companion object {
        /** Longer than RootViewModel keeps the session flow alive without subscribers. */
        const val STOPPED_LONG_ENOUGH_MILLIS = 6_000L
    }
}
