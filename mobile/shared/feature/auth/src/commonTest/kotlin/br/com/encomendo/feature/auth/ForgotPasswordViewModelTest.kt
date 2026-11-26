package br.com.encomendo.feature.auth

import app.cash.turbine.test
import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.domain.failure.InputIssue
import br.com.encomendo.core.domain.session.RequestPasswordReset
import br.com.encomendo.core.domain.session.ResetPassword
import br.com.encomendo.core.testing.FakePasswordResetRepository
import br.com.encomendo.core.ui.failure.FailureMessage
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import kotlin.test.AfterTest
import kotlin.test.BeforeTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

// The flow and wording are the design document's "Esqueci a senha", the same as the web's.
@OptIn(ExperimentalCoroutinesApi::class) // setMain / resetMain / advanceUntilIdle
class ForgotPasswordViewModelTest {
    private val main = StandardTestDispatcher()

    @BeforeTest
    fun useTestMain() = Dispatchers.setMain(main)

    @AfterTest
    fun restoreMain() = Dispatchers.resetMain()

    @Test
    fun anEmptyEmailIsMarkedAndNothingIsSent() =
        runTest(main) {
            val resets = FakePasswordResetRepository()
            val viewModel = forgotPassword(resets)

            viewModel.onSendCode()
            advanceUntilIdle()

            assertEquals(FieldProblem.Missing, viewModel.uiState.value.emailIssue)
            assertEquals(FormError.CheckFields, viewModel.uiState.value.error)
            assertEquals(emptyList(), resets.requested)
        }

    @Test
    fun sendsTheCodeThenAsksForIt() =
        runTest(main) {
            // The same words whether or not the account exists, as the API answers (ADR 0011).
            val resets = FakePasswordResetRepository()
            val viewModel = codeSent(resets)

            val state = viewModel.uiState.value
            assertEquals("ana@altarpm.com.br", state.sentTo)
            assertEquals(ResetNotice.CodeSent, state.notice)
            assertFalse(state.submitting)
            assertEquals(listOf("ana@altarpm.com.br"), resets.requested)
        }

    @Test
    fun aCodeThatIsNotSixDigitsAndAnEmptyPasswordAreMarked() =
        runTest(main) {
            val resets = FakePasswordResetRepository()
            val viewModel = codeSent(resets)
            viewModel.onCodeChange("123")

            viewModel.onChangePassword()
            advanceUntilIdle()

            val state = viewModel.uiState.value
            assertEquals(FieldProblem.Missing, state.codeIssue)
            assertEquals(FieldProblem.Missing, state.passwordIssue)
            assertEquals(FormError.CheckFields, state.error)
            assertEquals(emptyList(), resets.resets)
        }

    @Test
    fun aCodeCopiedWithSpacesCounts() =
        runTest(main) {
            val resets = FakePasswordResetRepository()
            val viewModel = codeSent(resets)
            viewModel.onCodeChange("482 913")
            viewModel.onPasswordChange("nova-senha-forte")

            viewModel.onChangePassword()
            advanceUntilIdle()

            assertEquals(listOf(Triple("ana@altarpm.com.br", "482913", "nova-senha-forte")), resets.resets)
        }

    @Test
    fun aChangedPasswordGoesBackToTheLoginOnce() =
        runTest(main) {
            val viewModel = codeSent(FakePasswordResetRepository())
            viewModel.onCodeChange("482913")
            viewModel.onPasswordChange("nova-senha-forte")

            viewModel.effects.test {
                viewModel.onChangePassword()
                assertEquals(ForgotPasswordEffect.PasswordChanged, awaitItem())
            }
            assertTrue(viewModel.uiState.value.submitting, "stays busy until the screen goes")
        }

    @Test
    fun aWrongCodeIsMarkedOnTheCode() =
        runTest(main) {
            val viewModel =
                codeSent(
                    FakePasswordResetRepository(resetAnswer = {
                        _,
                        _,
                        _,
                        ->
                        throw Failure.WrongCode()
                    }),
                )
            viewModel.onCodeChange("000000")
            viewModel.onPasswordChange("nova-senha-forte")

            viewModel.onChangePassword()
            advanceUntilIdle()

            val state = viewModel.uiState.value
            assertEquals(FieldProblem.Wrong, state.codeIssue)
            assertEquals(FormError.CheckFields, state.error)
            assertFalse(state.submitting)
        }

    @Test
    fun aPasswordTheServerRefusedShowsItsWords() =
        runTest(main) {
            val issues = listOf(InputIssue(field = "password", message = "A senha precisa de pelo menos 8 caracteres."))
            val viewModel =
                codeSent(
                    FakePasswordResetRepository(resetAnswer = {
                        _,
                        _,
                        _,
                        ->
                        throw Failure.InvalidInput(issues)
                    }),
                )
            viewModel.onCodeChange("482913")
            viewModel.onPasswordChange("curta")

            viewModel.onChangePassword()
            advanceUntilIdle()

            assertEquals(
                FieldProblem.Server("A senha precisa de pelo menos 8 caracteres."),
                viewModel.uiState.value.passwordIssue,
            )
        }

    @Test
    fun anotherCodeGoesToTheSameEmail() =
        runTest(main) {
            val resets = FakePasswordResetRepository()
            val viewModel = codeSent(resets)

            viewModel.onSendAnotherCode()
            advanceUntilIdle()

            assertEquals(listOf("ana@altarpm.com.br", "ana@altarpm.com.br"), resets.requested)
            assertEquals(ResetNotice.CodeResent, viewModel.uiState.value.notice)
        }

    @Test
    fun anotherEmailStartsOver() =
        runTest(main) {
            val viewModel = codeSent(FakePasswordResetRepository())
            viewModel.onCodeChange("482913")
            viewModel.onPasswordChange("nova-senha-forte")

            viewModel.onUseAnotherEmail()

            val state = viewModel.uiState.value
            assertNull(state.sentTo)
            assertNull(state.notice)
            assertEquals("", state.code)
            assertEquals("", state.password)
            assertEquals("ana@altarpm.com.br", state.email, "kept, to fix a typo")
        }

    @Test
    fun aRateLimitShowsTheWait() =
        runTest(main) {
            val viewModel =
                forgotPassword(FakePasswordResetRepository(requestAnswer = { throw Failure.RateLimited(60) }))
            viewModel.onEmailChange("ana@altarpm.com.br")

            viewModel.onSendCode()
            advanceUntilIdle()

            assertEquals(FormError.General(FailureMessage.RateLimited(60)), viewModel.uiState.value.error)
            assertNull(viewModel.uiState.value.sentTo)
        }

    @Test
    fun neverPrintsWhatWasTyped() =
        runTest(main) {
            val viewModel = codeSent(FakePasswordResetRepository())
            viewModel.onCodeChange("482913")
            viewModel.onPasswordChange("nova-senha-forte")

            val printed = viewModel.uiState.value.toString()
            listOf(
                "ana@altarpm.com.br",
                "482913",
                "nova-senha-forte",
            ).forEach { assertFalse(printed.contains(it), printed) }
        }

    @Test
    fun theCodeGoesToTheTrimmedEmail() =
        runTest(main) {
            val resets = FakePasswordResetRepository()
            val viewModel = forgotPassword(resets)
            viewModel.onEmailChange("  ana@altarpm.com.br ")

            viewModel.onSendCode()
            advanceUntilIdle()

            assertEquals("ana@altarpm.com.br", viewModel.uiState.value.sentTo)
            assertEquals(listOf("ana@altarpm.com.br"), resets.requested)
        }

    @Test
    fun aSecondTapOnEnviarCodigoIsIgnored() =
        runTest(main) {
            val answered = CompletableDeferred<Unit>()
            val resets = FakePasswordResetRepository(requestAnswer = { answered.await() })
            val viewModel = forgotPassword(resets)
            viewModel.onEmailChange("ana@altarpm.com.br")

            viewModel.onSendCode()
            viewModel.onSendCode()
            advanceUntilIdle()

            assertTrue(viewModel.uiState.value.submitting)
            assertEquals(1, resets.requested.size)
            answered.complete(Unit)
        }

    @Test
    fun aSecondTapOnAlterarSenhaIsIgnoredAndTheStepStays() =
        runTest(main) {
            val answered = CompletableDeferred<Unit>()
            val resets = FakePasswordResetRepository(resetAnswer = { _, _, _ -> answered.await() })
            val viewModel = codeSent(resets)
            viewModel.onCodeChange("482913")
            viewModel.onPasswordChange("nova-senha-forte")

            viewModel.onChangePassword()
            viewModel.onChangePassword()
            viewModel.onUseAnotherEmail()
            advanceUntilIdle()

            assertEquals(1, resets.resets.size)
            assertEquals("ana@altarpm.com.br", viewModel.uiState.value.sentTo, "Usar outro e-mail waits for the answer")
            answered.complete(Unit)
        }

    @Test
    fun anotherCodeClearsTheMarksOfTheEarlierOne() =
        runTest(main) {
            // As the web does: the earlier code stopped working, so "Código errado…" no longer applies.
            val viewModel =
                codeSent(
                    FakePasswordResetRepository(resetAnswer = {
                        _,
                        _,
                        _,
                        ->
                        throw Failure.WrongCode()
                    }),
                )
            viewModel.onCodeChange("000000")
            viewModel.onPasswordChange("nova-senha-forte")
            viewModel.onChangePassword()
            advanceUntilIdle()

            viewModel.onSendAnotherCode()
            advanceUntilIdle()

            val state = viewModel.uiState.value
            assertNull(state.codeIssue)
            assertNull(state.error)
            assertEquals(ResetNotice.CodeResent, state.notice)
        }

    @Test
    fun aFailedResendKeepsTheCodeStep() =
        runTest(main) {
            val resets = FakePasswordResetRepository()
            val viewModel = codeSent(resets)
            resets.requestAnswer = { throw Failure.Offline() }

            viewModel.onSendAnotherCode()
            advanceUntilIdle()

            val state = viewModel.uiState.value
            assertEquals("ana@altarpm.com.br", state.sentTo)
            assertEquals(ResetNotice.CodeSent, state.notice, "the first code still works")
            assertEquals(FormError.General(FailureMessage.Offline), state.error)
            assertFalse(state.submitting)
        }

    private fun forgotPassword(resets: FakePasswordResetRepository): ForgotPasswordViewModel =
        ForgotPasswordViewModel(RequestPasswordReset(resets), ResetPassword(resets))

    private fun TestScope.codeSent(resets: FakePasswordResetRepository): ForgotPasswordViewModel =
        forgotPassword(resets).apply {
            onEmailChange("ana@altarpm.com.br")
            onSendCode()
            advanceUntilIdle()
        }
}
