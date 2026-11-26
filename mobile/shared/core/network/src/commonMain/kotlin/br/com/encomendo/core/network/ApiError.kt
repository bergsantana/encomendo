package br.com.encomendo.core.network

import kotlin.jvm.JvmInline

/**
 * ApiError is thrown for every non-2xx response of the Encomendo API and for
 * requests that never reached it. It mirrors `ApiError` in web/packages/api.
 *
 * Branch on [code], never on [title]: codes are stable, titles are wording.
 * The message carries only the status and code, so logging it leaks nothing.
 */
class ApiError(
    /** The HTTP status, or 0 when the server could not be reached. */
    val status: Int,
    /** The stable `code` of the problem response. */
    val code: ApiErrorCode,
    /** The pt-BR summary the server sent, or null when the body was not a problem; the UI then picks its own text. */
    val title: String? = null,
    /** Optional pt-BR detail the server sent. */
    val detail: String? = null,
    /** The invalid fields of a 400 response, in the order the server sent them. */
    val fieldIssues: List<FieldIssue> = emptyList(),
    /** The wait `Retry-After` asked for, in whole seconds; null when the answer gave none or not in seconds. */
    val retryAfterSeconds: Int? = null,
    cause: Throwable? = null,
) : Exception("HTTP $status ${code.value}", cause)

/**
 * ApiErrorCode is the stable `code` of a problem+json response
 * (docs/api/openapi.yaml, `Problem`). New codes may appear on the server at any
 * time, so this is an open value rather than an enum: unknown codes are kept
 * as they arrive.
 */
@JvmInline
value class ApiErrorCode(
    /** The code exactly as the API sends it. */
    val value: String,
) {
    /**
     * The codes the contract lists today (docs/api/openapi.yaml, `Problem.code`),
     * plus the two the client adds itself.
     */
    companion object {
        /** The request body or parameters were invalid; see [ApiError.fieldIssues]. */
        val Invalid = ApiErrorCode("invalid")

        /** The resource does not exist in this tenant. */
        val NotFound = ApiErrorCode("not_found")

        /** The route exists but not with this method. */
        val MethodNotAllowed = ApiErrorCode("method_not_allowed")

        /** The write conflicts with the current state. */
        val Conflict = ApiErrorCode("conflict")

        /** The signed-in user may not do this. */
        val Forbidden = ApiErrorCode("forbidden")

        /** The server failed. */
        val Internal = ApiErrorCode("internal")

        /** A dependency of the server is down. */
        val NotReady = ApiErrorCode("not_ready")

        /** The access or refresh token is missing, bad or expired. */
        val Unauthenticated = ApiErrorCode("unauthenticated")

        /** Login or invitation acceptance: wrong email or password. A form error, never a reason to refresh. */
        val InvalidCredentials = ApiErrorCode("invalid_credentials")

        /** The session has no store selected; open the store picker. */
        val TenantRequired = ApiErrorCode("tenant_required")

        /** Someone saved first; reload and apply the change again. */
        val VersionConflict = ApiErrorCode("version_conflict")

        /** Too many attempts; see [ApiError.retryAfterSeconds]. */
        val RateLimited = ApiErrorCode("rate_limited")

        /** Sign-up: an account with this email already exists. */
        val EmailTaken = ApiErrorCode("email_taken")

        /** Creating a store: another store has this address. */
        val SlugTaken = ApiErrorCode("slug_taken")

        /** Inviting or accepting an invitation: the person is already on the team. */
        val AlreadyMember = ApiErrorCode("already_member")

        /** Password reset: the code is wrong, expired or used up. */
        val InvalidCode = ApiErrorCode("invalid_code")

        /** Inviting: the invitation was saved but the email could not be sent; invite again. */
        val EmailUnavailable = ApiErrorCode("email_unavailable")

        /** Client side: the request never reached the server (offline, timeout, DNS). */
        val Network = ApiErrorCode("network")

        /** Client side: the error body was not a problem response, or had no code. */
        val Unknown = ApiErrorCode("unknown")
    }
}

/** FieldIssue is one invalid field of a 400 problem response. */
data class FieldIssue(
    /** The request field, as named in the contract. */
    val field: String,
    /** pt-BR text the UI may show next to the field. */
    val message: String,
)

/**
 * True when this answer to `/auth/refresh` or `/auth/switch-tenant` means the
 * server looked at the refresh token and refused it: 400, 401 or 403. Only
 * these end the session; offline, 429 and 5xx keep it (ADR 0012, ADR 0013).
 */
val ApiError.refusesRefreshToken: Boolean get() = status in SESSION_ENDING_STATUSES

private val SESSION_ENDING_STATUSES = setOf(400, 401, 403)
