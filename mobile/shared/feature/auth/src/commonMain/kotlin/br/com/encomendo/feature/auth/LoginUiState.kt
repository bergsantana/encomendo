package br.com.encomendo.feature.auth

/**
 * LoginUiState is the login form: what was typed, whether a login is running
 * and what went wrong. [emailIssue] and [passwordIssue] are shown under their
 * fields; [error] above the form. [passwordVisible] shows the password as typed.
 */
data class LoginUiState(
    val email: String = "",
    val password: String = "",
    val submitting: Boolean = false,
    val error: FormError? = null,
    val emailIssue: FieldProblem? = null,
    val passwordIssue: FieldProblem? = null,
    val passwordVisible: Boolean = false,
) {
    /** Whether Entrar can be tapped: always, except while a login runs. Empty fields are marked on tap. */
    val canSubmit: Boolean get() = !submitting

    /** Leaves out the email and password, so a logged state carries no personal data. */
    override fun toString(): String =
        "LoginUiState(submitting=$submitting, error=$error, emailIssue=${emailIssue != null}, " +
            "passwordIssue=${passwordIssue != null}, passwordVisible=$passwordVisible)"
}
