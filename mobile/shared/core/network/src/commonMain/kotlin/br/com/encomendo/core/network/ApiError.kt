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
    /** The rows a refusal names (the kits of `item_in_kit`, the archived items of `item_archived`). */
    val references: List<ErrorReference> = emptyList(),
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

        /**
         * The selected store is suspended (ADR 0022): every admin call and a switch into it answer 403 with
         * this code. Show the store's message and go back to the store picker.
         */
        val TenantSuspended = ApiErrorCode("tenant_suspended")

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

        /** Catalog: another row of this kind has the name, ignoring case. */
        val NameTaken = ApiErrorCode("name_taken")

        /** Catalog: items still use the category being deleted. */
        val CategoryInUse = ApiErrorCode("category_in_use")

        /** Catalog: the item being archived is in kits that are not archived; see [ApiError.references]. */
        val ItemInKit = ApiErrorCode("item_in_kit")

        /** Catalog: an item of the kit, or one of its alternatives, is archived; see [ApiError.references]. */
        val ItemArchived = ApiErrorCode("item_archived")

        /** Photos: the item or kit already has 8. */
        val PhotoLimit = ApiErrorCode("photo_limit")

        /** Photos: the stored object is over 10 MB or not a JPEG, PNG or WebP image. */
        val PhotoInvalid = ApiErrorCode("photo_invalid")

        /** Photos: nothing was uploaded to the key, or it is not one issued for this item or kit. */
        val PhotoNotUploaded = ApiErrorCode("photo_not_uploaded")

        /** Client side: the upload to the storage host failed or was refused. */
        val UploadFailed = ApiErrorCode("upload_failed")

        /** Client side: the request never reached the server (offline, timeout, DNS). */
        val Network = ApiErrorCode("network")

        /** Client side: the error body was not a problem response, or had no code. */
        val Unknown = ApiErrorCode("unknown")
    }
}

/** ErrorReference is a row a refusal names, such as a kit that holds an item. */
data class ErrorReference(
    /** The row's id (a UUID). */
    val id: String,
    /** The row's name. */
    val name: String,
)

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
