package br.com.encomendo.feature.auth

import app.cash.turbine.test
import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.domain.failure.InputIssue
import br.com.encomendo.core.domain.session.Session
import br.com.encomendo.core.domain.session.SignUp
import br.com.encomendo.core.testing.FakeAuthRepository
import br.com.encomendo.core.testing.testSession
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

// The wording is the design document's "Criar conta (sign-up)", the same as the web's.
@OptIn(ExperimentalCoroutinesApi::class) // setMain / resetMain / runCurrent / advanceUntilIdle
class SignUpViewModelTest {
    private val main = StandardTestDispatcher()

    @BeforeTest
    fun useTestMain() = Dispatchers.setMain(main)

    @AfterTest
    fun restoreMain() = Dispatchers.resetMain()

    @Test
    fun emptyFieldsAreMarkedAndNothingIsSent() =
        runTest(main) {
            val auth = FakeAuthRepository()
            val viewModel = SignUpViewModel(SignUp(auth))
            viewModel.onNameChange("  ")

            viewModel.onSubmit()
            advanceUntilIdle()

            val state = viewModel.uiState.value
            assertEquals(FieldProblem.Missing, state.nameIssue)
            assertEquals(FieldProblem.Missing, state.emailIssue)
            assertEquals(FieldProblem.Missing, state.passwordIssue)
            assertEquals(FormError.CheckFields, state.error)
            assertEquals(emptyList(), auth.signUps)
        }

    @Test
    fun createsTheAccountWithWhatWasTyped() =
        runTest(main) {
            val auth = FakeAuthRepository()
            val viewModel = filledIn(auth)

            viewModel.onSubmit()
            advanceUntilIdle()

            assertEquals(listOf(Triple("Ana Souza", "ana@altarpm.com.br", "segredo-forte")), auth.signUps)
        }

    @Test
    fun staysBusyUntilTheScreenGoesAndIgnoresMoreTaps() =
        runTest(main) {
            // The root then opens the store picker and clears this ViewModel; until then nothing flickers.
            val answer = CompletableDeferred<Session>()
            val auth = FakeAuthRepository(signUpAnswer = { _, _, _ -> answer.await() })
            val viewModel = filledIn(auth)

            viewModel.uiState.test {
                assertFalse(awaitItem().submitting)
                viewModel.onSubmit()
                assertTrue(awaitItem().submitting)
                viewModel.onSubmit()
                answer.complete(testSession(tenant = null))
                runCurrent()
                expectNoEvents()
            }
            assertEquals(1, auth.signUps.size)
        }

    @Test
    fun anEmailInUseIsMarkedOnTheEmailField() =
        runTest(main) {
            val viewModel = failingWith(Failure.AlreadyTaken(field = "email"))

            viewModel.onSubmit()
            advanceUntilIdle()

            val state = viewModel.uiState.value
            assertEquals(FieldProblem.Taken, state.emailIssue)
            assertEquals(FormError.CheckFields, state.error)
            assertFalse(state.submitting)
        }

    @Test
    fun fieldsTheServerRefusedShowItsWords() =
        runTest(main) {
            // The password rule ("Pelo menos 8 caracteres.") is the server's to enforce; it says which field.
            val issues = listOf(InputIssue(field = "password", message = "A senha precisa de pelo menos 8 caracteres."))
            val viewModel = failingWith(Failure.InvalidInput(issues))

            viewModel.onSubmit()
            advanceUntilIdle()

            val state = viewModel.uiState.value
            assertEquals(FieldProblem.Server("A senha precisa de pelo menos 8 caracteres."), state.passwordIssue)
            assertNull(state.nameIssue)
            assertEquals(FormError.CheckFields, state.error)
        }

    @Test
    fun noSignalSaysSo() =
        runTest(main) {
            val viewModel = failingWith(Failure.Offline())

            viewModel.onSubmit()
            advanceUntilIdle()

            assertEquals(FormError.Offline, viewModel.uiState.value.error)
        }

    @Test
    fun aRateLimitShowsTheWaitAndKeepsItWhileTyping() =
        runTest(main) {
            val viewModel = failingWith(Failure.RateLimited(retryAfterSeconds = 30))
            viewModel.onSubmit()
            advanceUntilIdle()

            viewModel.onNameChange("Ana")

            assertEquals(FormError.RateLimited(retryAfterSeconds = 30), viewModel.uiState.value.error)
        }

    @Test
    fun typingClearsItsFieldAndTheBannerOnceNoneIsLeft() =
        runTest(main) {
            val viewModel = SignUpViewModel(SignUp(FakeAuthRepository()))
            viewModel.onSubmit()

            viewModel.onNameChange("Ana")
            viewModel.onEmailChange("ana@altarpm.com.br")
            assertEquals(FormError.CheckFields, viewModel.uiState.value.error, "the password is still marked")

            viewModel.onPasswordChange("segredo-forte")
            assertNull(viewModel.uiState.value.error)
        }

    @Test
    fun showsAndHidesThePassword() =
        runTest(main) {
            val viewModel = filledIn(FakeAuthRepository())

            viewModel.onPasswordVisibilityToggle()

            assertTrue(viewModel.uiState.value.passwordVisible)
        }

    @Test
    fun neverPrintsWhatWasTyped() =
        runTest(main) {
            val printed = filledIn(FakeAuthRepository()).uiState.value.toString()

            listOf(
                "Ana Souza",
                "ana@altarpm.com.br",
                "segredo-forte",
            ).forEach { assertFalse(printed.contains(it), printed) }
        }

    private fun failingWith(failure: Failure): SignUpViewModel =
        filledIn(FakeAuthRepository(signUpAnswer = { _, _, _ -> throw failure }))

    private fun filledIn(auth: FakeAuthRepository): SignUpViewModel =
        SignUpViewModel(SignUp(auth)).apply {
            onNameChange("Ana Souza")
            onEmailChange("ana@altarpm.com.br")
            onPasswordChange("segredo-forte")
        }
}
