package br.com.encomendo.feature.auth

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.domain.session.RequestPasswordReset
import br.com.encomendo.core.domain.session.ResetPassword
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.receiveAsFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/**
 * ForgotPasswordViewModel holds "Esqueci a senha": ask for a code by email,
 * then set a new password with it. One call at a time. Once the password has
 * changed it sends [ForgotPasswordEffect.PasswordChanged] and stays busy
 * until the screen goes.
 */
class ForgotPasswordViewModel(
    private val requestPasswordReset: RequestPasswordReset,
    private val resetPassword: ResetPassword,
) : ViewModel() {
    private val state = MutableStateFlow(ForgotPasswordUiState())
    private val once = Channel<ForgotPasswordEffect>(Channel.BUFFERED)

    /** The screen as it shows. */
    val uiState: StateFlow<ForgotPasswordUiState> = state.asStateFlow()

    /** What the screen does once (mobile/CLAUDE.md, Layers). */
    val effects: Flow<ForgotPasswordEffect> = once.receiveAsFlow()

    /** The user typed the email. */
    fun onEmailChange(email: String) =
        state.update { it.copy(email = email, emailIssue = null, error = it.error.afterTyping(null)) }

    /** The user typed the code. */
    fun onCodeChange(code: String) =
        state.update { it.copy(code = code, codeIssue = null, error = it.error.afterTyping(it.passwordIssue)) }

    /** The user typed the new password. */
    fun onPasswordChange(password: String) =
        state.update { it.copy(password = password, passwordIssue = null, error = it.error.afterTyping(it.codeIssue)) }

    /** The user tapped Mostrar or Ocultar on the new password. */
    fun onPasswordVisibilityToggle() = state.update { it.copy(passwordVisible = !it.passwordVisible) }

    /** The user tapped Enviar código. Marks an empty email instead of sending. */
    fun onSendCode() {
        val form = state.value
        if (form.submitting) return
        if (form.email.isBlank()) {
            state.update { it.copy(emailIssue = FieldProblem.Missing, error = FormError.CheckFields) }
            return
        }
        sendCode(email = form.email.trim(), notice = ResetNotice.CodeSent)
    }

    /** The user tapped Enviar outro código: a new code to the same email; the earlier one stops working. */
    fun onSendAnotherCode() {
        val form = state.value
        val email = form.sentTo ?: return
        if (form.submitting) return
        sendCode(email = email, notice = ResetNotice.CodeResent)
    }

    /** The user tapped Usar outro e-mail: back to the first step, keeping the email to fix a typo. */
    fun onUseAnotherEmail() {
        if (state.value.submitting) return
        state.update { ForgotPasswordUiState(email = it.email) }
    }

    /** The user tapped Alterar senha. Marks a code that isn't 6 digits, or an empty password, instead of sending. */
    fun onChangePassword() {
        val form = state.value
        val email = form.sentTo.takeUnless { form.submitting } ?: return
        val codeIssue = FieldProblem.Missing.takeUnless { ResetPassword.isWellFormed(form.code) }
        val passwordIssue = FieldProblem.Missing.takeIf { form.password.isEmpty() }
        if (codeIssue != null || passwordIssue != null) {
            state.update {
                it.copy(
                    codeIssue = codeIssue,
                    passwordIssue = passwordIssue,
                    error = FormError.CheckFields,
                )
            }
            return
        }
        state.update { it.copy(submitting = true, error = null, codeIssue = null, passwordIssue = null) }
        viewModelScope.launch {
            try {
                resetPassword(email = email, code = form.code, password = form.password)
                // Stays busy: the screen goes back to the login.
                once.send(ForgotPasswordEffect.PasswordChanged)
            } catch (failure: Failure) {
                state.update { it.showingReset(failure) }
            }
        }
    }

    private fun sendCode(
        email: String,
        notice: ResetNotice,
    ) {
        // Every mark goes, as on the web: after a new code, "Código errado…" no longer applies.
        state.update {
            it.copy(
                submitting = true,
                error = null,
                emailIssue = null,
                codeIssue = null,
                passwordIssue = null,
            )
        }
        viewModelScope.launch {
            try {
                requestPasswordReset(email)
                state.update { it.copy(submitting = false, sentTo = email, notice = notice) }
            } catch (failure: Failure) {
                state.update { it.showingRequest(failure) }
            }
        }
    }
}

private fun ForgotPasswordUiState.showingRequest(failure: Failure): ForgotPasswordUiState {
    val stopped = copy(submitting = false)
    // On a resend the email field is not on screen, so only the banner can say it.
    return if (failure is Failure.InvalidInput && sentTo == null) {
        stopped.copy(emailIssue = failure.problemFor(EMAIL_FIELD), error = FormError.CheckFields)
    } else {
        stopped.copy(error = failure.toFormError())
    }
}

private fun ForgotPasswordUiState.showingReset(failure: Failure): ForgotPasswordUiState {
    val stopped = copy(submitting = false)
    return when (failure) {
        is Failure.WrongCode -> stopped.copy(codeIssue = FieldProblem.Wrong, error = FormError.CheckFields)
        is Failure.InvalidInput ->
            stopped.copy(
                codeIssue = failure.problemFor(CODE_FIELD),
                passwordIssue = failure.problemFor(PASSWORD_FIELD),
                error = FormError.CheckFields,
            )
        else -> stopped.copy(error = failure.toFormError())
    }
}

/** The field names of the reset requests in docs/api/openapi.yaml. */
private const val EMAIL_FIELD = "email"
private const val CODE_FIELD = "code"
private const val PASSWORD_FIELD = "password"
