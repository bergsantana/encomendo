package br.com.encomendo.feature.auth

import app.cash.turbine.test
import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.domain.failure.InputIssue
import br.com.encomendo.core.domain.session.LogIn
import br.com.encomendo.core.domain.session.Session
import br.com.encomendo.core.testing.FakeAuthRepository
import br.com.encomendo.core.testing.testSession
import br.com.encomendo.core.ui.failure.FailureMessage
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import kotlin.test.AfterTest
import kotlin.test.BeforeTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

// The wording is the design document's "Account and store screens (staff)", the same as the web's.
@OptIn(ExperimentalCoroutinesApi::class) // setMain / resetMain / runCurrent / advanceUntilIdle
class LoginViewModelTest {
    private val main = StandardTestDispatcher()

    @BeforeTest
    fun useTestMain() = Dispatchers.setMain(main)

    @AfterTest
    fun restoreMain() = Dispatchers.resetMain()

    @Test
    fun emptyFieldsAreMarkedAndNothingIsSent() =
        runTest(main) {
            val auth = FakeAuthRepository()
            val viewModel = LoginViewModel(LogIn(auth))
            viewModel.onEmailChange("   ")

            viewModel.onSubmit()
            advanceUntilIdle()

            val state = viewModel.uiState.value
            assertEquals(FieldProblem.Missing, state.emailIssue)
            assertEquals(FieldProblem.Missing, state.passwordIssue)
            assertEquals(FormError.CheckFields, state.error)
            assertEquals(emptyList(), auth.logins)
        }

    @Test
    fun onlyTheEmptyFieldIsMarked() =
        runTest(main) {
            val viewModel = LoginViewModel(LogIn(FakeAuthRepository()))
            viewModel.onEmailChange("ana@altarpm.com.br")

            viewModel.onSubmit()

            assertNull(viewModel.uiState.value.emailIssue)
            assertEquals(FieldProblem.Missing, viewModel.uiState.value.passwordIssue)
        }

    @Test
    fun typingInAMarkedFieldClearsOnlyItAndTheBannerOnceNoneIsLeft() =
        runTest(main) {
            // The banner and the marked fields must agree: it stays while any field is still marked.
            val viewModel = LoginViewModel(LogIn(FakeAuthRepository()))
            viewModel.onSubmit()

            viewModel.onEmailChange("ana@altarpm.com.br")
            assertNull(viewModel.uiState.value.emailIssue)
            assertEquals(FieldProblem.Missing, viewModel.uiState.value.passwordIssue)
            assertEquals(FormError.CheckFields, viewModel.uiState.value.error)

            viewModel.onPasswordChange("segredo-forte")
            assertNull(viewModel.uiState.value.error)
        }

    @Test
    fun aChangedPasswordIsAnnouncedUntilTheNextAttempt() =
        runTest(main) {
            // "Senha alterada. Entre com a nova senha." after Esqueci a senha (design document).
            val viewModel = filledIn(FakeAuthRepository(answer = { _, _ -> throw Failure.Offline() }))

            viewModel.onPasswordChanged()
            assertEquals(LoginNotice.PasswordChanged, viewModel.uiState.value.notice)

            viewModel.onSubmit()
            assertNull(viewModel.uiState.value.notice)
        }

    @Test
    fun aChangedPasswordIsForgottenOnceTheLoginIsLeft() =
        runTest(main) {
            // As the web does: the news was for this visit, not for a return from Criar conta.
            val viewModel = LoginViewModel(LogIn(FakeAuthRepository()))
            viewModel.onPasswordChanged()

            viewModel.onLeave()

            assertNull(viewModel.uiState.value.notice)
        }

    @Test
    fun submitsTheTypedCredentials() =
        runTest(main) {
            val auth = FakeAuthRepository()
            val viewModel = filledIn(auth)

            viewModel.onSubmit()
            advanceUntilIdle()

            assertEquals(listOf("ana@altarpm.com.br" to "segredo-forte"), auth.logins)
        }

    @Test
    fun showsProgressAndIgnoresMoreTapsWhileTheLoginRuns() =
        runTest(main) {
            // Gloved hands tap twice; a second login would only waste an attempt of the rate limit.
            val answer = CompletableDeferred<Session>()
            val auth = FakeAuthRepository(answer = { _, _ -> answer.await() })
            val viewModel = filledIn(auth)

            viewModel.onSubmit()
            runCurrent()
            assertTrue(viewModel.uiState.value.submitting)
            assertFalse(viewModel.uiState.value.canSubmit)

            viewModel.onSubmit()
            answer.complete(testSession())
            advanceUntilIdle()

            assertEquals(1, auth.logins.size)
        }

    @Test
    fun aSuccessfulLoginStaysBusyUntilTheScreenGoes() =
        runTest(main) {
            // The root swaps to the management graph on its own and clears this ViewModel. Until then
            // the form must not come back enabled or empty: that flickers, and an emptied password
            // field stops Android from offering to save the password.
            val viewModel = filledIn(FakeAuthRepository())

            viewModel.uiState.test {
                assertFalse(awaitItem().submitting)
                viewModel.onSubmit()
                val busy = awaitItem()
                assertTrue(busy.submitting)
                assertEquals("segredo-forte", busy.password)
                advanceUntilIdle()
                expectNoEvents()
            }
        }

    @Test
    fun wrongCredentialsShowAFormErrorAndKeepWhatWasTyped() =
        runTest(main) {
            val viewModel = failingWith(Failure.WrongCredentials())

            viewModel.onSubmit()
            advanceUntilIdle()

            val state = viewModel.uiState.value
            assertEquals(FormError.WrongCredentials, state.error)
            assertEquals("ana@altarpm.com.br", state.email)
            assertEquals("segredo-forte", state.password)
            assertFalse(state.submitting)
            assertTrue(state.canSubmit)
        }

    @Test
    fun noSignalSaysSo() =
        runTest(main) {
            val viewModel = failingWith(Failure.Offline())

            viewModel.onSubmit()
            advanceUntilIdle()

            assertEquals(FormError.General(FailureMessage.Offline), viewModel.uiState.value.error)
        }

    @Test
    fun aRateLimitShowsTheWait() =
        runTest(main) {
            val viewModel = failingWith(Failure.RateLimited(retryAfterSeconds = 30))

            viewModel.onSubmit()
            advanceUntilIdle()

            assertEquals(
                FormError.General(FailureMessage.RateLimited(retryAfterSeconds = 30)),
                viewModel.uiState.value.error,
            )
        }

    @Test
    fun fieldsTheServerRefusedShowItsWordsAndTheFormSaysToCheckThem() =
        runTest(main) {
            val issues =
                listOf(
                    InputIssue(field = "email", message = "Informe um e-mail válido."),
                    InputIssue(field = "password", message = "A senha passa do limite."),
                )
            val viewModel = failingWith(Failure.InvalidInput(issues))

            viewModel.onSubmit()
            advanceUntilIdle()

            val state = viewModel.uiState.value
            assertEquals(FieldProblem.Server("Informe um e-mail válido."), state.emailIssue)
            assertEquals(FieldProblem.Server("A senha passa do limite."), state.passwordIssue)
            assertEquals(FormError.CheckFields, state.error)
        }

    @Test
    fun aServerFailureSaysTheServerDidNotAnswer() =
        runTest(main) {
            val viewModel = failingWith(Failure.Unexpected(status = 503, code = "not_ready", title = "Indisponível"))

            viewModel.onSubmit()
            advanceUntilIdle()

            assertEquals(FormError.General(FailureMessage.ServerUnavailable), viewModel.uiState.value.error)
        }

    @Test
    fun anyOtherRefusalShowsTheServersOwnWords() =
        runTest(main) {
            val viewModel = failingWith(Failure.Unexpected(status = 409, code = "conflict", title = "Conta bloqueada"))

            viewModel.onSubmit()
            advanceUntilIdle()

            assertEquals(FormError.General(FailureMessage.Refused("Conta bloqueada")), viewModel.uiState.value.error)
        }

    @Test
    fun aFailureWithNothingToSayIsAGeneralError() =
        runTest(main) {
            // A full disk or a broken Keystore: no server words, and the server is fine, so don't blame it.
            val viewModel = failingWith(Failure.Unexpected(status = 0, code = "storage"))

            viewModel.onSubmit()
            advanceUntilIdle()

            assertEquals(FormError.General(FailureMessage.Unexpected), viewModel.uiState.value.error)
        }

    @Test
    fun editingAFieldClearsItsIssueAndTheFormError() =
        runTest(main) {
            val issues = listOf(InputIssue(field = "email", message = "Informe um e-mail válido."))
            val auth = FakeAuthRepository(answer = { _, _ -> throw Failure.InvalidInput(issues) })
            val viewModel = filledIn(auth)
            viewModel.onSubmit()
            advanceUntilIdle()

            viewModel.onPasswordChange("outra")
            assertEquals(
                FieldProblem.Server("Informe um e-mail válido."),
                viewModel.uiState.value.emailIssue,
                "only its own field clears",
            )

            viewModel.onEmailChange("ana@altarpm.com.br")
            assertNull(viewModel.uiState.value.emailIssue)

            auth.answer = { _, _ -> throw Failure.WrongCredentials() }
            viewModel.onSubmit()
            advanceUntilIdle()
            viewModel.onPasswordChange("mais-uma")
            assertNull(viewModel.uiState.value.error)
        }

    @Test
    fun theRateLimitWaitStaysWhileTyping() =
        runTest(main) {
            // Typing doesn't shorten the wait, so the message mustn't vanish as if it had.
            val viewModel = failingWith(Failure.RateLimited(retryAfterSeconds = 30))
            viewModel.onSubmit()
            advanceUntilIdle()

            viewModel.onPasswordChange("outra")
            viewModel.onEmailChange("ana@altarpm.com.br ")

            assertEquals(
                FormError.General(FailureMessage.RateLimited(retryAfterSeconds = 30)),
                viewModel.uiState.value.error,
            )
        }

    @Test
    fun showsAndHidesThePassword() =
        runTest(main) {
            // Gloves make typos; seeing the password saves an attempt.
            val viewModel = filledIn(FakeAuthRepository())
            assertFalse(viewModel.uiState.value.passwordVisible)

            viewModel.onPasswordVisibilityToggle()
            assertTrue(viewModel.uiState.value.passwordVisible)

            viewModel.onPasswordVisibilityToggle()
            assertFalse(viewModel.uiState.value.passwordVisible)
        }

    @Test
    fun neverPrintsTheEmailOrPassword() =
        runTest(main) {
            val printed = filledIn(FakeAuthRepository()).uiState.value.toString()

            assertFalse(printed.contains("ana@altarpm.com.br"), printed)
            assertFalse(printed.contains("segredo-forte"), printed)
        }

    private fun failingWith(failure: Failure): LoginViewModel =
        filledIn(FakeAuthRepository(answer = { _, _ -> throw failure }))

    private fun filledIn(auth: FakeAuthRepository): LoginViewModel =
        LoginViewModel(LogIn(auth)).apply {
            onEmailChange("ana@altarpm.com.br")
            onPasswordChange("segredo-forte")
        }
}
