package br.com.encomendo.feature.auth

import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.ui.failure.FailureMessage
import br.com.encomendo.core.ui.failure.toFailureMessage

/**
 * FormError is a problem with a whole staff form, shown above it. Each case
 * has the exact pt-BR text of the design document's "Account and store
 * screens (staff)", the same on the web.
 */
sealed interface FormError {
    /** A field is empty or was refused: "Confira os campos destacados." Each field says what. */
    data object CheckFields : FormError

    /** Login: the email and password match no account. */
    data object WrongCredentials : FormError

    /** Store picker: the user no longer belongs to the store they chose. */
    data object NotAMember : FormError

    /** Any other failure, in the words every staff screen has for it: offline, a 5xx, a rate limit, a refusal. */
    data class General(
        val message: FailureMessage,
    ) : FormError
}

/** FieldProblem is what is wrong with one field, shown under it. */
sealed interface FieldProblem {
    /** Left empty; each screen words it for its field ("Informe seu e-mail."). */
    data object Missing : FieldProblem

    /** Someone else's already, such as a store's link; each screen words it for its field. */
    data object Taken : FieldProblem

    /** Refused for a reason the screen has words for, such as a wrong reset code. */
    data object Wrong : FieldProblem

    /** Refused by the server, in its own pt-BR words. */
    data class Server(
        val message: String,
    ) : FieldProblem
}

/**
 * Returns the form error every staff form shows for this failure. A form
 * that words a failure its own way (fields refused, a store no longer the
 * user's) handles it before calling this.
 */
internal fun Failure.toFormError(): FormError =
    when (this) {
        is Failure.InvalidInput, is Failure.AlreadyTaken, is Failure.WrongCode -> FormError.CheckFields
        is Failure.WrongCredentials -> FormError.WrongCredentials
        is Failure.Offline, is Failure.RateLimited, is Failure.Forbidden, is Failure.Unexpected ->
            FormError.General(toFailureMessage())
    }

/**
 * The form error to keep while the user types: a rate limit's wait, which
 * typing doesn't shorten, and "check the fields" while [stillMarked] is.
 */
internal fun FormError?.afterTyping(stillMarked: FieldProblem?): FormError? =
    when {
        this is FormError.General && message is FailureMessage.RateLimited -> this
        this == FormError.CheckFields -> takeIf { stillMarked != null }
        else -> null
    }

/** The server's words for [field], if it refused that field. */
internal fun Failure.InvalidInput.problemFor(field: String): FieldProblem? =
    issues.firstOrNull { it.field == field }?.let { FieldProblem.Server(it.message) }
