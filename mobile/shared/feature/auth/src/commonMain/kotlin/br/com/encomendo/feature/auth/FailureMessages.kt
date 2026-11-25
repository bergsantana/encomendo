package br.com.encomendo.feature.auth

import androidx.compose.runtime.Composable
import br.com.encomendo.feature.auth.resources.Res
import br.com.encomendo.feature.auth.resources.login_error_rate_limited_minutes
import br.com.encomendo.feature.auth.resources.login_error_rate_limited_seconds
import org.jetbrains.compose.resources.pluralStringResource

/** True for a 5xx: the server's fault, worded as "O servidor não respondeu" as on the web. Status 0 is the device. */
internal fun isServerFailure(status: Int): Boolean = status >= FIRST_SERVER_ERROR

/** The rate-limit message with its wait, as the web words it (web/apps/admin/src/errorMessage.ts). */
@Composable
internal fun rateLimitMessage(retryAfterSeconds: Int?): String {
    // Every 429 carries Retry-After (ADR 0013); a minute is a safe guess if one ever does not, as on web.
    val seconds = retryAfterSeconds ?: SECONDS_PER_MINUTE
    val minutes = minutesToWait(seconds)
    return if (minutes == null) {
        pluralStringResource(Res.plurals.login_error_rate_limited_seconds, seconds, seconds)
    } else {
        pluralStringResource(Res.plurals.login_error_rate_limited_minutes, minutes, minutes)
    }
}

/**
 * Returns the wait in whole minutes, rounded up, from a minute on; null below
 * that, where seconds read better. The web admin uses the same thresholds.
 * Rounding up never tells the user to come back before the limit resets.
 */
internal fun minutesToWait(seconds: Int): Int? =
    if (seconds < MINUTES_FROM_SECONDS) null else (seconds + SECONDS_PER_MINUTE - 1) / SECONDS_PER_MINUTE

private const val FIRST_SERVER_ERROR = 500
private const val MINUTES_FROM_SECONDS = 60
private const val SECONDS_PER_MINUTE = 60
