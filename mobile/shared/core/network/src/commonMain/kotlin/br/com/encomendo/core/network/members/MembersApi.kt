package br.com.encomendo.core.network.members

import br.com.encomendo.core.network.decoded
import io.ktor.client.HttpClient
import io.ktor.client.request.delete
import io.ktor.client.request.get
import io.ktor.client.request.parameter
import io.ktor.client.request.put
import io.ktor.client.request.setBody
import io.ktor.http.ContentType
import io.ktor.http.contentType
import io.ktor.http.encodeURLPathPart
import kotlinx.serialization.Serializable

/**
 * MembersApi calls the team's endpoints (docs/api/openapi.yaml, `members`).
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

    // Encoded, so an id can only ever name one member, never another path.
    private fun memberPath(userId: String): String = "$MEMBERS/${userId.encodeURLPathPart()}"

    private companion object {
        const val MEMBERS = "/api/v1/admin/members"
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

@Serializable
private class ChangeRoleRequest(
    val role: String,
)
