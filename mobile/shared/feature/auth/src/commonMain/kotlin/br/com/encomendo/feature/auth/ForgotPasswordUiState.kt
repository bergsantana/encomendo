package br.com.encomendo.feature.auth

/**
 * ForgotPasswordUiState is "Esqueci a senha", in two steps. Until a code is
 * sent ([sentTo] is null) it asks for the email; then for the code and a new
 * password. [notice] says where the code went, the same whether or not the
 * account exists (ADR 0011).
 */
data class ForgotPasswordUiState(
    val email: String = "",
    val sentTo: String? = null,
    val notice: ResetNotice? = null,
    val code: String = "",
    val password: String = "",
    val passwordVisible: Boolean = false,
    val submitting: Boolean = false,
    val error: FormError? = null,
    val emailIssue: FieldProblem? = null,
    val codeIssue: FieldProblem? = null,
    val passwordIssue: FieldProblem? = null,
) {
    /** Leaves out the email, code and password, so a logged state carries no personal data. */
    override fun toString(): String =
        "ForgotPasswordUiState(codeSent=${sentTo != null}, notice=$notice, submitting=$submitting, error=$error, " +
            "emailIssue=${emailIssue != null}, codeIssue=${codeIssue != null}, passwordIssue=${passwordIssue != null})"
}

/** ResetNotice says where the code went. */
enum class ResetNotice {
    /** The first code: "Se houver uma conta com {e-mail}, enviamos um código de 6 dígitos." */
    CodeSent,

    /** Another one: "… enviamos um novo código. O anterior deixou de valer." */
    CodeResent,
}

/** ForgotPasswordEffect is what the screen does once. */
sealed interface ForgotPasswordEffect {
    /** The password changed: back to the login, which says so. */
    data object PasswordChanged : ForgotPasswordEffect
}
