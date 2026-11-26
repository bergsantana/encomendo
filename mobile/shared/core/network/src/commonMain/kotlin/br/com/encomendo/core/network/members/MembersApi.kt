package br.com.encomendo.core.network.members

import br.com.encomendo.core.network.decoded
import io.ktor.client.HttpClient
import io.ktor.client.request.delete
import io.ktor.client.request.get
import io.ktor.client.request.parameter
import io.ktor.client.request.post
import io.ktor.client.request.put
import io.ktor.client.request.setBody
import io.ktor.http.ContentType
import io.ktor.http.contentType
import io.ktor.http.encodeURLPathPart
import kotlinx.serialization.Serializable

/**
 * MembersApi calls the team's and its invitations' endpoints
 * (docs/api/openapi.yaml, `members`).
 * Calls carry the session's access token and need `members:manage`; an
 * answer that isn't 2xx, or a 2xx that isn't what the contract says, throws
 * ApiError.
 */
class MembersApi(
    private val client: HttpClient,
) {
    /** Returns up to [limit] members, oldest first, from [cursor] on; null [cursor] reads the first page. */
    suspend fun list(
        cursor: String?,
        limit: Int,
    ): MemberPageDto =
        client
            .get(MEMBERS) {
                parameter("cursor", cursor)
                parameter("limit", limit)
            }.decoded()

    /** Gives [userId] the role [role] (admin, staff or viewer) and returns the member as saved. */
    suspend fun changeRole(
        userId: String,
        role: String,
    ): MemberDto =
        client
            .put("${memberPath(userId)}/role") {
                contentType(ContentType.Application.Json)
                setBody(ChangeRoleRequest(role = role))
            }.decoded()

    /** Takes [userId] off the team; the answer is 204 with no body. */
    suspend fun remove(userId: String) {
        client.delete(memberPath(userId))
    }

    /**
     * Returns up to [limit] pending invitations, newest first, from [cursor]
     * on; null [cursor] reads the first page.
     */
    suspend fun invitations(
        cursor: String?,
        limit: Int,
    ): InvitationPageDto =
        client
            .get(INVITATIONS) {
                parameter("cursor", cursor)
                parameter("limit", limit)
            }.decoded()

    /**
     * Emails [email] an invitation with [role] (admin, staff or viewer) and
     * returns it; a pending one for the same email is replaced. The link is
     * only in the email, never in the answer.
     */
    suspend fun invite(
        email: String,
        role: String,
    ): InvitationDto =
        client
            .post(INVITATIONS) {
                contentType(ContentType.Application.Json)
                setBody(InviteRequest(email = email, role = role))
            }.decoded()

    /** Cancels [invitationId]; the answer is 204 with no body. */
    suspend fun revoke(invitationId: String) {
        client.delete("$INVITATIONS/${invitationId.encodeURLPathPart()}")
    }

    // Encoded, so an id can only ever name one member, never another path.
    private fun memberPath(userId: String): String = "$MEMBERS/${userId.encodeURLPathPart()}"

    private companion object {
        const val MEMBERS = "/api/v1/admin/members"
        const val INVITATIONS = "/api/v1/admin/invitations"
    }
}

/** MemberDto is a team member as the API sends it. Not a data class, so logs never carry a name or email. */
@Serializable
class MemberDto(
    val userId: String,
    val name: String,
    val email: String,
    val role: String,
)

/** MemberPageDto is one page of `GET /api/v1/admin/members`; [nextCursor] is absent on the last page. */
@Serializable
class MemberPageDto(
    val items: List<MemberDto>,
    val nextCursor: String? = null,
)

/** InvitationDto is a pending invitation as the API sends it. Not a data class, so logs never carry an email. */
@Serializable
class InvitationDto(
    val id: String,
    val email: String,
    val role: String,
    /** An RFC 3339 date-time. */
    val expiresAt: String,
)

/** InvitationPageDto is one page of `GET /api/v1/admin/invitations`; [nextCursor] is absent on the last page. */
@Serializable
class InvitationPageDto(
    val items: List<InvitationDto>,
    val nextCursor: String? = null,
)

@Serializable
private class InviteRequest(
    val email: String,
    val role: String,
)

@Serializable
private class ChangeRoleRequest(
    val role: String,
)
