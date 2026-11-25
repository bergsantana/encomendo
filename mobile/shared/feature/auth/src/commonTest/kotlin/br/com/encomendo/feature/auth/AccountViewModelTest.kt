package br.com.encomendo.feature.auth

import app.cash.turbine.ReceiveTurbine
import app.cash.turbine.test
import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.domain.session.LogOut
import br.com.encomendo.core.domain.session.Membership
import br.com.encomendo.core.domain.session.ObserveSession
import br.com.encomendo.core.domain.session.Role
import br.com.encomendo.core.domain.session.Session
import br.com.encomendo.core.domain.session.TenantSummary
import br.com.encomendo.core.testing.AltaRpm
import br.com.encomendo.core.testing.FakeAuthRepository
import br.com.encomendo.core.testing.FakeSessionRepository
import br.com.encomendo.core.testing.testSession
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import kotlin.test.AfterTest
import kotlin.test.BeforeTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

@OptIn(ExperimentalCoroutinesApi::class) // setMain / resetMain / runCurrent
class AccountViewModelTest {
    private val main = StandardTestDispatcher()

    @BeforeTest
    fun useTestMain() = Dispatchers.setMain(main)

    @AfterTest
    fun restoreMain() = Dispatchers.resetMain()

    @Test
    fun showsWhoIsSignedInAndInWhichStore() =
        runTest(main) {
            // As the web menu's foot does: the person and the store, so a shared phone is never a surprise.
            val viewModel = accountViewModel()

            viewModel.uiState.test {
                val state = skipToSignedIn()
                assertEquals("Ana Souza", state.userName)
                assertEquals(AltaRpm.name, state.storeName)
            }
        }

    @Test
    fun offersToSwitchStoresOnlyWithTwoOrMore() =
        runTest(main) {
            // As the web's menu: "Trocar de loja" with two or more stores.
            val motoPecas =
                TenantSummary(id = "0190f3c4-0000-7000-8000-000000000002", name = "Moto Peças", slug = "moto-pecas")
            val one = testSession()
            val two = one.copy(memberships = one.memberships + Membership(motoPecas, Role("staff")))

            accountViewModel(session = one).uiState.test { assertFalse(skipToSignedIn().canSwitchStore) }
            accountViewModel(session = two).uiState.test { assertTrue(skipToSignedIn().canSwitchStore) }
        }

    @Test
    fun opensAndClosesTheMenu() =
        runTest(main) {
            val viewModel = accountViewModel()

            viewModel.uiState.test {
                skipToSignedIn()
                viewModel.onMenuOpen()
                assertTrue(awaitItem().menuOpen)
                viewModel.onMenuDismiss()
                assertFalse(awaitItem().menuOpen)
            }
        }

    @Test
    fun logsOutOnceAndStaysBusyUntilTheScreenGoes() =
        runTest(main) {
            // The root swaps to the login once the session is gone and clears this ViewModel.
            val leaving = CompletableDeferred<Unit>()
            val auth = FakeAuthRepository(logOutAnswer = { leaving.await() })
            val viewModel = accountViewModel(auth)

            viewModel.uiState.test {
                skipToSignedIn()
                viewModel.onMenuOpen()
                awaitItem()
                viewModel.onLogOut()
                assertTrue(awaitItem().loggingOut)
                viewModel.onLogOut()
                leaving.complete(Unit)
                runCurrent()
                expectNoEvents()
            }
            assertEquals(1, auth.logouts)
        }

    @Test
    fun aLogoutThatFailsSaysSoInTheMenu() =
        runTest(main) {
            val disk = CompletableDeferred<Unit>()
            val auth =
                FakeAuthRepository(logOutAnswer = {
                    disk.await()
                    throw Failure.Unexpected(status = 0, code = "storage")
                })
            val viewModel = accountViewModel(auth)

            viewModel.uiState.test {
                skipToSignedIn()
                viewModel.onMenuOpen()
                awaitItem()
                viewModel.onLogOut()
                assertTrue(awaitItem().loggingOut)
                viewModel.onMenuDismiss() // closed while the logout ran: the error must still show
                assertFalse(awaitItem().menuOpen)

                disk.complete(Unit)
                val failed = awaitItem()
                assertTrue(failed.logOutFailed)
                assertTrue(failed.menuOpen)
                assertFalse(failed.loggingOut)

                viewModel.onMenuDismiss()
                assertFalse(awaitItem().logOutFailed, "the error goes with the menu")
            }
        }

    @Test
    fun neverPrintsTheUsersName() =
        runTest(main) {
            val viewModel = accountViewModel()

            viewModel.uiState.test {
                val printed = skipToSignedIn().toString()
                assertFalse(printed.contains("Ana Souza"), printed)
            }
        }

    private fun accountViewModel(
        auth: FakeAuthRepository = FakeAuthRepository(),
        session: Session = testSession(),
    ): AccountViewModel = AccountViewModel(ObserveSession(FakeSessionRepository(session)), LogOut(auth))

    /** Skips the empty state before the session is read; returns the first signed-in state. */
    private suspend fun ReceiveTurbine<AccountUiState>.skipToSignedIn(): AccountUiState {
        var state = awaitItem()
        while (state.userName.isEmpty()) state = awaitItem()
        return state
    }
}
