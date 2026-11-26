package br.com.encomendo.feature.auth

import br.com.encomendo.core.domain.failure.Failure

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

    /** The call never reached the server. */
    data object Offline : FormError

    /** The server failed or did not answer (a 5xx). */
    data object ServerUnavailable : FormError

    /** Too many attempts; [retryAfterSeconds] is the wait, or null when the server gave none. */
    data class RateLimited(
        val retryAfterSeconds: Int?,
    ) : FormError

    /** Any other refusal, in the server's own pt-BR words. */
    data class Refused(
        val title: String,
    ) : FormError

    /** Nothing better to say: a failure on the device, or an answer the app cannot use. */
    data object Unexpected : FormError
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
        is Failure.Offline -> FormError.Offline
        is Failure.RateLimited -> FormError.RateLimited(retryAfterSeconds)
        is Failure.InvalidInput, is Failure.AlreadyTaken, is Failure.WrongCode -> FormError.CheckFields
        is Failure.WrongCredentials -> FormError.WrongCredentials
        is Failure.Forbidden -> refusedIn(title)
        is Failure.Unexpected -> if (isServerFailure(status)) FormError.ServerUnavailable else refusedIn(title)
    }

/** The server's own words, or a general error when it sent none: never an empty banner. */
private fun refusedIn(title: String?): FormError =
    title?.takeIf { it.isNotBlank() }?.let(FormError::Refused) ?: FormError.Unexpected

/**
 * The form error to keep while the user types: a rate limit's wait, which
 * typing doesn't shorten, and "check the fields" while [stillMarked] is.
 */
internal fun FormError?.afterTyping(stillMarked: FieldProblem?): FormError? =
    when (this) {
        is FormError.RateLimited -> this
        FormError.CheckFields -> takeIf { stillMarked != null }
        else -> null
    }

/** The server's words for [field], if it refused that field. */
internal fun Failure.InvalidInput.problemFor(field: String): FieldProblem? =
    issues.firstOrNull { it.field == field }?.let { FieldProblem.Server(it.message) }
