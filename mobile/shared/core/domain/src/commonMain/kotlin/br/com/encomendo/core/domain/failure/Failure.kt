package br.com.encomendo.core.domain.failure

/**
 * Failure is what a repository throws when it cannot do what was asked. It is
 * the one error type ViewModels catch (mobile/CLAUDE.md, Kotlin style), and
 * each kind says what the user can do about it. Messages carry only codes,
 * never what the user typed, so logging a Failure leaks nothing.
 */
sealed class Failure(
    message: String,
    cause: Throwable?,
) : Exception(message, cause) {
    /** The request never reached the server: no signal, a timeout, a DNS failure. Try again later. */
    class Offline(
        cause: Throwable? = null,
    ) : Failure("offline", cause)

    /** Too many attempts. [retryAfterSeconds] is the wait the server asked for, or null when it gave none. */
    class RateLimited(
        val retryAfterSeconds: Int?,
        cause: Throwable? = null,
    ) : Failure("rate limited", cause)

    /** The input breaks a rule; [issues] lists the fields in the server's own pt-BR words. */
    class InvalidInput(
        val issues: List<InputIssue>,
        cause: Throwable? = null,
    ) : Failure("invalid input", cause)

    /**
     * The signed-in user may not do this, such as opening a store they no
     * longer belong to. [title] is the API's pt-BR summary, null when none.
     */
    class Forbidden(
        val title: String? = null,
        cause: Throwable? = null,
    ) : Failure("forbidden", cause)

    /**
     * The store is suspended (ADR 0022), so nothing in it can be done or opened. Unlike
     * [Forbidden], the words are fixed by the design document, so no title is kept.
     */
    class StoreSuspended(
        cause: Throwable? = null,
    ) : Failure("store suspended", cause)

    /** The value of [field] (`email`, `slug` …) is someone else's already; the screen asks for another. */
    class AlreadyTaken(
        val field: String,
        cause: Throwable? = null,
    ) : Failure("already taken: $field", cause)

    /**
     * What the call was about is gone, such as a member someone else removed
     * meanwhile. [title] is the API's pt-BR summary, null when none.
     */
    class NotFound(
        val title: String? = null,
        cause: Throwable? = null,
    ) : Failure("not found", cause)

    /** Inviting someone who is on the store's team already. */
    class AlreadyMember(
        cause: Throwable? = null,
    ) : Failure("already member", cause)

    /**
     * The invitation was saved, but its email did not go out. Sending it
     * again replaces the link and tries the email once more.
     */
    class EmailNotSent(
        cause: Throwable? = null,
    ) : Failure("email not sent", cause)

    /** A password reset code that is wrong, expired, used up or never sent: the server says the same for each. */
    class WrongCode(
        cause: Throwable? = null,
    ) : Failure("wrong code", cause)

    /** The email and password match no account. The server never says which one was wrong. */
    class WrongCredentials(
        cause: Throwable? = null,
    ) : Failure("wrong credentials", cause)

    /** Someone saved the row first (`version_conflict`): read it again and apply the change again. */
    class VersionConflict(
        cause: Throwable? = null,
    ) : Failure("version conflict", cause)

    /** A category cannot be deleted while items, archived ones included, use it (`category_in_use`). */
    class CategoryInUse(
        cause: Throwable? = null,
    ) : Failure("category in use", cause)

    /** An item cannot be archived while kits that are not archived hold it (`item_in_kit`); [kits] names them. */
    class ItemInKit(
        val kits: List<Reference>,
        cause: Throwable? = null,
    ) : Failure("item in kit", cause)

    /** A kit cannot be saved or restored while an item in it is archived (`item_archived`); [items] names them. */
    class ItemArchived(
        val items: List<Reference>,
        cause: Throwable? = null,
    ) : Failure("item archived", cause)

    /** The item or kit has 8 photos already (`photo_limit`). */
    class PhotoLimit(
        cause: Throwable? = null,
    ) : Failure("photo limit", cause)

    /** The photo is over 10 MB or not a JPEG, PNG or WebP image (`photo_invalid`); the server deleted it. */
    class PhotoInvalid(
        cause: Throwable? = null,
    ) : Failure("photo invalid", cause)

    /**
     * Nothing was uploaded to the photo's key, or the upload to storage failed or expired
     * (`photo_not_uploaded`); pick the photo again.
     */
    class PhotoNotUploaded(
        cause: Throwable? = null,
    ) : Failure("photo not uploaded", cause)

    /**
     * Anything the screen has no better answer for: a server failure, an
     * answer the app cannot read, a refusal it does not expect. [status] is the
     * HTTP status, or 0 when the failure was on the device; [code] is the
     * API's problem code, kept for logs; [title] is the API's pt-BR summary,
     * which a screen shows for a refusal it has no words of its own for, and
     * null when there is none. The message keeps only the status and code.
     */
    class Unexpected(
        val status: Int,
        val code: String,
        val title: String? = null,
        cause: Throwable? = null,
    ) : Failure("unexpected: $status $code", cause)
}

/** InputIssue is one field the server refused, with pt-BR text the screen shows next to it. */
data class InputIssue(
    /** The request field, as named in the contract (`email`, `password` …). */
    val field: String,
    /** pt-BR text from the server. */
    val message: String,
)

/** Reference names a row a refusal is about, such as a kit that holds an item. */
data class Reference(
    /** The row's id (a UUID). */
    val id: String,
    /** The row's name. */
    val name: String,
)
