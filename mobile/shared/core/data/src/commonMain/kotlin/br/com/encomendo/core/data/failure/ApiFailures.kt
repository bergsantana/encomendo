package br.com.encomendo.core.data.failure

import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.domain.failure.InputIssue
import br.com.encomendo.core.network.ApiError
import br.com.encomendo.core.network.ApiErrorCode
import kotlinx.io.IOException

/**
 * Runs [call] and turns what the API client throws into a [Failure], the one
 * error type above core:data. An [IOException] from the device itself (a
 * full disk under DataStore, a Keystore that cannot seal) is a
 * [Failure.Unexpected] with status 0. CancellationException passes through
 * untouched.
 */
internal suspend fun <T> asFailures(call: suspend () -> T): T =
    try {
        call()
    } catch (error: ApiError) {
        throw error.toFailure()
    } catch (error: IOException) {
        throw Failure.Unexpected(status = 0, code = "storage", cause = error)
    }

/**
 * Maps an [ApiError] by its code, never its title (docs/api/openapi.yaml,
 * `Problem`). A 429 is a rate limit even without a problem body, as a proxy
 * may send one.
 */
internal fun ApiError.toFailure(): Failure =
    when {
        code == ApiErrorCode.Network -> Failure.Offline(cause = this)
        code == ApiErrorCode.RateLimited || status == TOO_MANY_REQUESTS ->
            Failure.RateLimited(retryAfterSeconds = retryAfterSeconds, cause = this)
        code == ApiErrorCode.Invalid ->
            Failure.InvalidInput(
                issues = fieldIssues.map { InputIssue(field = it.field, message = it.message) },
                cause = this,
            )
        code == ApiErrorCode.InvalidCredentials -> Failure.WrongCredentials(cause = this)
        code == ApiErrorCode.Forbidden -> Failure.Forbidden(title = title, cause = this)
        code == ApiErrorCode.NotFound -> Failure.NotFound(title = title, cause = this)
        code == ApiErrorCode.AlreadyMember -> Failure.AlreadyMember(cause = this)
        // A 503 like a server failure, but the invitation is saved: the screen says so.
        code == ApiErrorCode.EmailUnavailable -> Failure.EmailNotSent(cause = this)
        code == ApiErrorCode.SlugTaken -> Failure.AlreadyTaken(field = "slug", cause = this)
        code == ApiErrorCode.EmailTaken -> Failure.AlreadyTaken(field = "email", cause = this)
        code == ApiErrorCode.InvalidCode -> Failure.WrongCode(cause = this)
        else -> Failure.Unexpected(status = status, code = code.value, title = title, cause = this)
    }

private const val TOO_MANY_REQUESTS = 429
