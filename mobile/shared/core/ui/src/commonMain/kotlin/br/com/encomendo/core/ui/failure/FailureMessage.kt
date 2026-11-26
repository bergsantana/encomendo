package br.com.encomendo.core.ui.failure

import br.com.encomendo.core.domain.failure.Failure

/**
 * FailureMessage is what any staff screen says about a failure it has no
 * words of its own for. Each case has the exact pt-BR text of the design
 * document's "Messages any of these screens can show" ("Account and store
 * screens (staff)"), which the web admin's `errorMessage.ts` uses too.
 */
sealed interface FailureMessage {
    /** The call never reached the server. */
    data object Offline : FailureMessage

    /** The server failed or did not answer (a 5xx). */
    data object ServerUnavailable : FailureMessage

    /** Too many attempts; [retryAfterSeconds] is the wait, or null when the server gave none. */
    data class RateLimited(
        val retryAfterSeconds: Int?,
    ) : FailureMessage

    /** The store is suspended: "Esta loja está suspensa. Fale com o suporte do Encomendo." */
    data object StoreSuspended : FailureMessage

    /** Any other refusal, in the server's own pt-BR words. */
    data class Refused(
        val title: String,
    ) : FailureMessage

    /** Nothing better to say: a failure on the device, or an answer the app cannot use. */
    data object Unexpected : FailureMessage
}

/**
 * Returns the message any staff screen shows for this failure. Wrong input,
 * a taken value, a wrong code, wrong credentials, someone already on the
 * team and an invitation whose email did not go out are
 * [FailureMessage.Unexpected] here: a screen that can get them has words of
 * its own, next to its fields, and handles them before calling this.
 */
fun Failure.toFailureMessage(): FailureMessage =
    when (this) {
        is Failure.Offline -> FailureMessage.Offline
        is Failure.RateLimited -> FailureMessage.RateLimited(retryAfterSeconds)
        is Failure.StoreSuspended -> FailureMessage.StoreSuspended
        is Failure.Forbidden -> refusedIn(title)
        is Failure.NotFound -> refusedIn(title)
        is Failure.Unexpected -> if (isServerFailure) FailureMessage.ServerUnavailable else refusedIn(title)
        is Failure.InvalidInput,
        is Failure.AlreadyTaken,
        is Failure.WrongCode,
        is Failure.WrongCredentials,
        is Failure.AlreadyMember,
        is Failure.EmailNotSent,
        -> FailureMessage.Unexpected
    }

/** A 5xx is the server's fault, worded as "O servidor não respondeu" as on the web. Status 0 is the device. */
private val Failure.Unexpected.isServerFailure: Boolean get() = status >= FIRST_SERVER_ERROR

/** The server's own words, or a general error when it sent none: never an empty message. */
private fun refusedIn(title: String?): FailureMessage =
    title?.takeIf { it.isNotBlank() }?.let(FailureMessage::Refused) ?: FailureMessage.Unexpected

/**
 * Returns the wait in whole minutes, rounded up, from a minute on; null below
 * that, where seconds read better. The web admin uses the same thresholds.
 * Rounding up never tells the user to come back before the limit resets.
 */
internal fun minutesToWait(seconds: Int): Int? =
    if (seconds < MINUTES_FROM_SECONDS) null else (seconds + SECONDS_PER_MINUTE - 1) / SECONDS_PER_MINUTE

private const val FIRST_SERVER_ERROR = 500
private const val MINUTES_FROM_SECONDS = 60
internal const val SECONDS_PER_MINUTE = 60
