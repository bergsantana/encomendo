package br.com.encomendo.feature.auth

/**
 * LoginUiState is the login form: what was typed, whether a login is running
 * and what went wrong. [emailIssue] and [passwordIssue] are the server's
 * pt-BR words for that field; [error] is about the whole form.
 * [passwordVisible] shows the password as typed.
 */
data class LoginUiState(
    val email: String = "",
    val password: String = "",
    val submitting: Boolean = false,
    val error: LoginError? = null,
    val emailIssue: String? = null,
    val passwordIssue: String? = null,
    val passwordVisible: Boolean = false,
) {
    /** Whether Entrar is enabled: both fields filled in and no login running. */
    val canSubmit: Boolean get() = !submitting && email.isNotBlank() && password.isNotEmpty()

    /** Leaves out the email and password, so a logged state carries no personal data. */
    override fun toString(): String =
        "LoginUiState(submitting=$submitting, error=$error, emailIssue=${emailIssue != null}, " +
            "passwordIssue=${passwordIssue != null}, passwordVisible=$passwordVisible)"
}

/** LoginError is a problem with the whole form, shown above the Entrar button. */
sealed interface LoginError {
    /** The email and password match no account. */
    data object WrongCredentials : LoginError

    /** The login never reached the server. */
    data object Offline : LoginError

    /** Too many attempts; [retryAfterSeconds] is the wait, or null when the server gave none. */
    data class RateLimited(
        val retryAfterSeconds: Int?,
    ) : LoginError

    /** The server failed or did not answer (a 5xx). */
    data object ServerUnavailable : LoginError

    /** Anything else: an answer the app cannot use, or a failure on the device. */
    data object Unexpected : LoginError
}
