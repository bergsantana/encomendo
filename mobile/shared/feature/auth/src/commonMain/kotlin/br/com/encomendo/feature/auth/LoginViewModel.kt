package br.com.encomendo.feature.auth

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.domain.session.LogIn
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/**
 * LoginViewModel holds the login form. It sends one login at a time and
 * never navigates: once the session is stored, the root opens the
 * management graph by itself.
 */
class LoginViewModel(
    private val logIn: LogIn,
) : ViewModel() {
    private val state = MutableStateFlow(LoginUiState())

    /** The form as the screen shows it. */
    val uiState: StateFlow<LoginUiState> = state.asStateFlow()

    /** The user typed in the email field. Clears that field's issue, and the form error once no field is marked. */
    fun onEmailChange(email: String) =
        state.update { it.copy(email = email, emailIssue = null, error = it.error.afterTyping(it.passwordIssue)) }

    /** The user typed in the password field. Clears that field's issue, and the form error once no field is marked. */
    fun onPasswordChange(password: String) =
        state.update { it.copy(password = password, passwordIssue = null, error = it.error.afterTyping(it.emailIssue)) }

    /** The user tapped Mostrar or Ocultar on the password field. */
    fun onPasswordVisibilityToggle() = state.update { it.copy(passwordVisible = !it.passwordVisible) }

    /**
     * The user tapped Entrar, or Done on the keyboard. Marks empty fields
     * instead of sending; does nothing while a login runs.
     */
    fun onSubmit() {
        val form = state.value
        if (!form.canSubmit) return
        val emailIssue = FieldProblem.Missing.takeIf { form.email.isBlank() }
        val passwordIssue = FieldProblem.Missing.takeIf { form.password.isEmpty() }
        if (emailIssue != null || passwordIssue != null) {
            state.update {
                it.copy(
                    emailIssue = emailIssue,
                    passwordIssue = passwordIssue,
                    error = FormError.CheckFields,
                )
            }
            return
        }
        state.update { it.copy(submitting = true, error = null, emailIssue = null, passwordIssue = null) }
        viewModelScope.launch {
            try {
                logIn(email = form.email, password = form.password)
                // Stays submitting: the root swaps to the management graph and clears this
                // ViewModel. Re-enabling or emptying the form first would flicker, and an
                // emptied password field keeps Android from offering to save the password.
            } catch (failure: Failure) {
                state.update { it.showing(failure) }
            }
        }
    }
}

private fun LoginUiState.showing(failure: Failure): LoginUiState {
    val stopped = copy(submitting = false)
    return when (failure) {
        is Failure.InvalidInput ->
            stopped.copy(
                emailIssue = failure.problemFor(EMAIL_FIELD),
                passwordIssue = failure.problemFor(PASSWORD_FIELD),
                error = FormError.CheckFields,
            )
        else -> stopped.copy(error = failure.toFormError())
    }
}

/** The field names of `LoginRequest` in docs/api/openapi.yaml. */
private const val EMAIL_FIELD = "email"
private const val PASSWORD_FIELD = "password"
