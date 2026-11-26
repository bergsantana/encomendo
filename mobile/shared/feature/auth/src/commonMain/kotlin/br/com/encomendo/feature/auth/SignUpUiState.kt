package br.com.encomendo.feature.auth

/**
 * SignUpUiState is the sign-up form ("Criar conta"): what was typed, whether
 * the account is being created and what went wrong. Each field's issue shows
 * under it; [error] above the form.
 */
data class SignUpUiState(
    val name: String = "",
    val email: String = "",
    val password: String = "",
    val submitting: Boolean = false,
    val error: FormError? = null,
    val nameIssue: FieldProblem? = null,
    val emailIssue: FieldProblem? = null,
    val passwordIssue: FieldProblem? = null,
    val passwordVisible: Boolean = false,
) {
    /** Whether Criar conta can be tapped: always, except while it runs. Empty fields are marked on tap. */
    val canSubmit: Boolean get() = !submitting

    /** Leaves out what was typed, so a logged state carries no personal data. */
    override fun toString(): String =
        "SignUpUiState(submitting=$submitting, error=$error, nameIssue=${nameIssue != null}, " +
            "emailIssue=${emailIssue != null}, passwordIssue=${passwordIssue != null})"
}
