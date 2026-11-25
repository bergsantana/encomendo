package br.com.encomendo.core.network.auth

import kotlinx.serialization.Serializable

/**
 * AuthSessionDto is the `AuthSession` every sign-up, login, refresh and store
 * switch returns (docs/api/openapi.yaml). Map it to domain types in core:data.
 * Not a data class: [toString] must never print the tokens.
 */
@Serializable
class AuthSessionDto(
    /** The JWT for `Authorization: Bearer`. Never parsed: the session fields below say what it allows. */
    val accessToken: String,
    /**
     * The rotated refresh token. Optional in the contract, because the web gets
     * it as a cookie; Android always uses the body transport, so an answer
     * without it cannot be refreshed and fails to decode.
     */
    val refreshToken: String,
    /** The signed-in staff member. */
    val user: UserDto,
    /** The selected store; absent when none is selected. */
    val tenant: TenantSummaryDto? = null,
    /** The role in [tenant]; absent when none is selected. */
    val role: String? = null,
    /** The permission keys the role grants in [tenant]; empty when none is selected. */
    val permissions: List<String>,
    /** Every store the user belongs to. */
    val memberships: List<MembershipDto>,
) {
    override fun toString(): String = "AuthSessionDto(user=${user.id}, tenant=${tenant?.id}, role=$role)"
}

/** UserDto is the API's `User`. */
@Serializable
class UserDto(
    /** The account's id (a UUID). */
    val id: String,
    /** The display name. */
    val name: String,
    /** The login email. */
    val email: String,
)

/** TenantSummaryDto is the API's `TenantSummary`: one store. */
@Serializable
class TenantSummaryDto(
    /** The store's id (a UUID). */
    val id: String,
    /** The store's display name. */
    val name: String,
    /** The store page subdomain. */
    val slug: String,
)

/** MembershipDto is the API's `Membership`: a store and the user's role there. */
@Serializable
class MembershipDto(
    /** The store. */
    val tenant: TenantSummaryDto,
    /** The role key, for example `owner`. */
    val role: String,
)
