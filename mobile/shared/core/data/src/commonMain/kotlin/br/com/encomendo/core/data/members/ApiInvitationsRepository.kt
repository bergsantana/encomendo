package br.com.encomendo.core.data.members

import br.com.encomendo.core.data.failure.StoreCalls
import br.com.encomendo.core.domain.members.Invitation
import br.com.encomendo.core.domain.members.InvitationsRepository
import br.com.encomendo.core.domain.paging.Page
import br.com.encomendo.core.domain.session.Role
import br.com.encomendo.core.network.ApiError
import br.com.encomendo.core.network.ApiErrorCode
import br.com.encomendo.core.network.members.InvitationDto
import br.com.encomendo.core.network.members.MembersApi
import kotlin.time.Instant

/** ApiInvitationsRepository reads and sends the team's invitations through [api], in the session's store. */
class ApiInvitationsRepository(
    private val api: MembersApi,
    private val calls: StoreCalls,
) : InvitationsRepository {
    override suspend fun page(
        cursor: String?,
        limit: Int,
    ): Page<Invitation> =
        calls.inStore {
            val page = api.invitations(cursor = cursor, limit = limit)
            Page(items = page.items.map { it.toInvitation() }, nextCursor = page.nextCursor)
        }

    override suspend fun invite(
        email: String,
        role: Role,
    ): Invitation = calls.inStore { api.invite(email = email, role = role.key).toInvitation() }

    override suspend fun revoke(invitationId: String) = calls.inStore { api.revoke(invitationId = invitationId) }
}

private fun InvitationDto.toInvitation(): Invitation =
    Invitation(id = id, email = email, role = invitableRole(), expiresAt = parsedExpiry())

// An answer that breaks the contract is the API client's "unknown" failure, as a body that does not decode is.
private fun InvitationDto.parsedExpiry(): Instant =
    try {
        Instant.parse(expiresAt)
    } catch (_: IllegalArgumentException) {
        throw unusable()
    }

// The contract's InvitableRole: an invitation never makes an owner, and "Reenviar" sends the role back as it came.
private fun InvitationDto.invitableRole(): Role = Role(role).takeIf { it in Role.Invitable } ?: throw unusable()

private fun unusable() = ApiError(status = OK, code = ApiErrorCode.Unknown)

private const val OK = 200
