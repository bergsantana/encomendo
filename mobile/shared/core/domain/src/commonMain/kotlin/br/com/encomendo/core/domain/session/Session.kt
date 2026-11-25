package br.com.encomendo.core.domain.session

import kotlin.jvm.JvmInline

/**
 * Session is who is signed in and what they may do, as the API last described
 * it (`Session` in docs/api/openapi.yaml, ADR 0010). Every login, refresh and
 * store switch replaces it. The tokens that go with it stay in core:security.
 */
data class Session(
    /** The signed-in staff member. */
    val user: User,
    /** The selected store, or null when none is selected yet: right after sign-up, or with several stores. */
    val tenant: TenantSummary?,
    /** The role in [tenant]; null when no store is selected. */
    val role: Role?,
    /** What [role] allows in [tenant]; nothing when no store is selected. */
    val permissions: GrantedPermissions,
    /** Every store the user belongs to, for the store picker. */
    val memberships: List<Membership>,
)

/** User is a staff account: the person behind the session. [toString] prints only the id. */
data class User(
    /** The account's id (a UUID), stable across stores. */
    val id: String,
    /** The name the user signed up with. */
    val name: String,
    /** The login email. */
    val email: String,
) {
    // A session in a log line, crash report or failed test must not carry personal data.
    override fun toString(): String = "User(id=$id)"
}

/** TenantSummary is a store as the session and the store picker show it. */
data class TenantSummary(
    /** The store's id (a UUID); the API selects stores by it. */
    val id: String,
    /** The store's display name, for example "Alta RPM". */
    val name: String,
    /** The store page subdomain. */
    val slug: String,
)

/** Membership is one store the user belongs to, and the role there. */
data class Membership(
    /** The store. */
    val tenant: TenantSummary,
    /** The user's role in it. */
    val role: Role,
)

/**
 * Role is a staff member's role in one store, exactly as the API names it
 * (`owner`, `admin`, `staff`, `viewer`). The app shows what [Session.permissions]
 * allow; it never derives permissions from the role itself.
 */
@JvmInline
value class Role(
    /** The role key as the API sends it. */
    val key: String,
)
