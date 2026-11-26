package br.com.encomendo.core.domain.members

import br.com.encomendo.core.domain.paging.Page
import br.com.encomendo.core.domain.session.Role
import br.com.encomendo.core.domain.session.SessionRepository

/**
 * ListInvitations reads the pending invitations a page at a time. A refusal
 * renews the session first, as [ListMembers] does.
 */
class ListInvitations(
    private val invitations: InvitationsRepository,
    private val sessions: SessionRepository,
) {
    /** Returns the page that starts at [cursor], or the first page. */
    suspend operator fun invoke(cursor: String? = null): Page<Invitation> =
        sessions.renewingWhenRefused { invitations.page(cursor = cursor, limit = PAGE_SIZE) }

    private companion object {
        /** As for the team: one page is usually all of them, as on the web. */
        const val PAGE_SIZE = 100
    }
}

/**
 * InviteMember emails someone an invitation to the team, or sends a pending
 * one again with a new link and expiry ("Reenviar"). A refusal renews the
 * session first.
 */
class InviteMember(
    private val invitations: InvitationsRepository,
    private val sessions: SessionRepository,
) {
    /** Invites [email], trimmed, with [role], which is never [Role.Owner]: a store has one owner. */
    suspend operator fun invoke(
        email: String,
        role: Role,
    ): Invitation {
        require(role in Role.Invitable) { "only admin, staff or viewer can be invited" }
        return sessions.renewingWhenRefused { invitations.invite(email = email.trim(), role = role) }
    }
}

/** RevokeInvitation cancels a pending invitation; its link stops working. A refusal renews the session first. */
class RevokeInvitation(
    private val invitations: InvitationsRepository,
    private val sessions: SessionRepository,
) {
    /** Cancels [invitation]. */
    suspend operator fun invoke(invitation: Invitation) =
        sessions.renewingWhenRefused { invitations.revoke(invitationId = invitation.id) }
}
