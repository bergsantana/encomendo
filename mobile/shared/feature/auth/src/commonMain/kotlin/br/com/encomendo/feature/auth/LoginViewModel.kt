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

    /** The user typed in the email field. Clears that field's issue and the form error, except a wait. */
    fun onEmailChange(email: String) =
        state.update { it.copy(email = email, emailIssue = null, error = it.errorAfterTyping()) }

    /** The user typed in the password field. Clears that field's issue and the form error, except a wait. */
    fun onPasswordChange(password: String) =
        state.update { it.copy(password = password, passwordIssue = null, error = it.errorAfterTyping()) }

    /** The user tapped Mostrar or Ocultar on the password field. */
    fun onPasswordVisibilityToggle() = state.update { it.copy(passwordVisible = !it.passwordVisible) }

    /** The user tapped Entrar, or Done on the keyboard. Does nothing while a login runs or a field is empty. */
    fun onSubmit() {
        val form = state.value
        if (!form.canSubmit) return
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

/** Typing fixes a wrong field, but doesn't shorten a rate limit's wait. */
private fun LoginUiState.errorAfterTyping(): LoginError? = error as? LoginError.RateLimited

private fun LoginUiState.showing(failure: Failure): LoginUiState {
    val stopped = copy(submitting = false)
    return when (failure) {
        is Failure.WrongCredentials -> stopped.copy(error = LoginError.WrongCredentials)
        is Failure.Offline -> stopped.copy(error = LoginError.Offline)
        is Failure.RateLimited -> stopped.copy(error = LoginError.RateLimited(failure.retryAfterSeconds))
        is Failure.InvalidInput -> {
            val email = failure.issues.firstOrNull { it.field == EMAIL_FIELD }?.message
            val password = failure.issues.firstOrNull { it.field == PASSWORD_FIELD }?.message
            // An issue on a field the form doesn't show can't be fixed here: say something went wrong.
            if (email == null && password == null) {
                stopped.copy(error = LoginError.Unexpected)
            } else {
                stopped.copy(emailIssue = email, passwordIssue = password)
            }
        }
        is Failure.Unexpected -> stopped.copy(error = unexpected(failure.status))
        // Not answers a login gives; say something went wrong rather than guess.
        is Failure.Forbidden, is Failure.AlreadyTaken -> stopped.copy(error = LoginError.Unexpected)
    }
}

private fun unexpected(status: Int): LoginError =
    if (isServerFailure(status)) LoginError.ServerUnavailable else LoginError.Unexpected

/** The field names of `LoginRequest` in docs/api/openapi.yaml. */
private const val EMAIL_FIELD = "email"
private const val PASSWORD_FIELD = "password"
