package br.com.encomendo.core.domain.members

import br.com.encomendo.core.domain.paging.Page
import br.com.encomendo.core.domain.session.Role

/**
 * InvitationsRepository reads and sends the selected store's invitations.
 * Every call needs `members:manage`, checked against the caller's current
 * role (ADR 0011); without it, it throws `Failure.Forbidden`.
 */
interface InvitationsRepository {
    /** Returns up to [limit] pending invitations, newest first, from [cursor] on (null for the first page). */
    suspend fun page(
        cursor: String?,
        limit: Int,
    ): Page<Invitation>

    /**
     * Emails [email] an invitation to join with [role] and returns it. A
     * pending invitation for the same email is replaced, with a new link and
     * expiry: that is how "Reenviar" works. Throws `Failure.AlreadyMember`
     * when the person is on the team, and `Failure.EmailNotSent` when the
     * invitation was saved but its email did not go out.
     */
    suspend fun invite(
        email: String,
        role: Role,
    ): Invitation

    /** Cancels [invitationId]; its link stops working. Throws `Failure.NotFound` when it is no longer pending. */
    suspend fun revoke(invitationId: String)
}
