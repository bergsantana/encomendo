package br.com.encomendo.core.ui.failure

import androidx.compose.runtime.Composable
import br.com.encomendo.core.ui.resources.Res
import br.com.encomendo.core.ui.resources.failure_offline
import br.com.encomendo.core.ui.resources.failure_rate_limited_minutes
import br.com.encomendo.core.ui.resources.failure_rate_limited_seconds
import br.com.encomendo.core.ui.resources.failure_server
import br.com.encomendo.core.ui.resources.failure_unexpected
import org.jetbrains.compose.resources.pluralStringResource
import org.jetbrains.compose.resources.stringResource

/** The pt-BR text of [this] message, in the design document's words. */
@Composable
fun FailureMessage.text(): String =
    when (this) {
        FailureMessage.Offline -> stringResource(Res.string.failure_offline)
        FailureMessage.ServerUnavailable -> stringResource(Res.string.failure_server)
        FailureMessage.Unexpected -> stringResource(Res.string.failure_unexpected)
        is FailureMessage.Refused -> title
        is FailureMessage.RateLimited -> rateLimitText(retryAfterSeconds)
    }

/** The rate-limit message with its wait, as the web words it (web/apps/admin/src/errorMessage.ts). */
@Composable
private fun rateLimitText(retryAfterSeconds: Int?): String {
    // Every 429 carries Retry-After (ADR 0013); a minute is a safe guess if one ever does not, as on web.
    val seconds = retryAfterSeconds ?: SECONDS_PER_MINUTE
    val minutes = minutesToWait(seconds)
    return if (minutes == null) {
        pluralStringResource(Res.plurals.failure_rate_limited_seconds, seconds, seconds)
    } else {
        pluralStringResource(Res.plurals.failure_rate_limited_minutes, minutes, minutes)
    }
}
