package br.com.encomendo.feature.auth

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.domain.session.SignUp
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/**
 * SignUpViewModel holds the sign-up form. It creates one account at a time
 * and never navigates: the new account has no store, so the root opens the
 * store picker ("Crie sua loja") by itself.
 */
class SignUpViewModel(
    private val signUp: SignUp,
) : ViewModel() {
    private val state = MutableStateFlow(SignUpUiState())

    /** The form as the screen shows it. */
    val uiState: StateFlow<SignUpUiState> = state.asStateFlow()

    /** The user typed their name. */
    fun onNameChange(name: String) =
        state.update {
            val stillMarked = it.emailIssue ?: it.passwordIssue
            it.copy(name = name, nameIssue = null, error = it.error.afterTyping(stillMarked))
        }

    /** The user typed their email. */
    fun onEmailChange(email: String) =
        state.update {
            val stillMarked = it.nameIssue ?: it.passwordIssue
            it.copy(email = email, emailIssue = null, error = it.error.afterTyping(stillMarked))
        }

    /** The user typed a password. */
    fun onPasswordChange(password: String) =
        state.update {
            val stillMarked = it.nameIssue ?: it.emailIssue
            it.copy(password = password, passwordIssue = null, error = it.error.afterTyping(stillMarked))
        }

    /** The user tapped Mostrar or Ocultar on the password field. */
    fun onPasswordVisibilityToggle() = state.update { it.copy(passwordVisible = !it.passwordVisible) }

    /** The user tapped Criar conta. Marks empty fields instead of sending; does nothing while it runs. */
    fun onSubmit() {
        val form = state.value
        if (!form.canSubmit) return
        val nameIssue = FieldProblem.Missing.takeIf { form.name.isBlank() }
        val emailIssue = FieldProblem.Missing.takeIf { form.email.isBlank() }
        val passwordIssue = FieldProblem.Missing.takeIf { form.password.isEmpty() }
        if (nameIssue != null || emailIssue != null || passwordIssue != null) {
            state.update {
                it.copy(
                    nameIssue = nameIssue,
                    emailIssue = emailIssue,
                    passwordIssue = passwordIssue,
                    error = FormError.CheckFields,
                )
            }
            return
        }
        state.update {
            it.copy(
                submitting = true,
                error = null,
                nameIssue = null,
                emailIssue = null,
                passwordIssue = null,
            )
        }
        viewModelScope.launch {
            try {
                signUp(name = form.name, email = form.email, password = form.password)
                // Stays submitting: the root swaps to the store picker and clears this ViewModel.
            } catch (failure: Failure) {
                state.update { it.showing(failure) }
            }
        }
    }
}

private fun SignUpUiState.showing(failure: Failure): SignUpUiState {
    val stopped = copy(submitting = false)
    return when (failure) {
        is Failure.AlreadyTaken -> {
            val emailTaken = failure.field == EMAIL_FIELD
            stopped.copy(emailIssue = if (emailTaken) FieldProblem.Taken else emailIssue, error = FormError.CheckFields)
        }
        is Failure.InvalidInput ->
            stopped.copy(
                nameIssue = failure.problemFor(NAME_FIELD),
                emailIssue = failure.problemFor(EMAIL_FIELD),
                passwordIssue = failure.problemFor(PASSWORD_FIELD),
                error = FormError.CheckFields,
            )
        else -> stopped.copy(error = failure.toFormError())
    }
}

/** The field names of `SignUpRequest` in docs/api/openapi.yaml. */
private const val NAME_FIELD = "name"
private const val EMAIL_FIELD = "email"
private const val PASSWORD_FIELD = "password"
