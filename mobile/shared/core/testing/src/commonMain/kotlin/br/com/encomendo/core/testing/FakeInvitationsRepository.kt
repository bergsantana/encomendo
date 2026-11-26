package br.com.encomendo.core.testing

import br.com.encomendo.core.domain.members.Invitation
import br.com.encomendo.core.domain.members.InvitationsRepository
import br.com.encomendo.core.domain.paging.Page
import br.com.encomendo.core.domain.session.Role
import kotlin.time.Instant

/**
 * FakeInvitationsRepository keeps the [pending] invitations in memory and
 * records every call. By default it answers as the API would: all of them in
 * one page; an invitation saved first, in place of a pending one for the same
 * email, with a new id; one cancelled. Set [pageAnswer], [inviteAnswer] or
 * [revokeAnswer] to page differently, wait, or throw a `Failure`.
 */
class FakeInvitationsRepository(
    pending: List<Invitation> = emptyList(),
) : InvitationsRepository {
    /** The pending invitations as the server holds them now, newest first. */
    var pending: List<Invitation> = pending

    /** Answers a page request; all of [pending] by default. */
    var pageAnswer: suspend (cursor: String?) -> Page<Invitation> = { Page(items = this.pending, nextCursor = null) }

    /** Answers an invitation; saves it first in [pending] by default. */
    var inviteAnswer: suspend (email: String, role: Role) -> Invitation = { email, role ->
        val sent = Invitation(id = "invitation-${++sentCount}", email = email, role = role, expiresAt = NEXT_WEEK)
        this.pending = listOf(sent) + this.pending.filter { it.key != sent.key }
        sent
    }

    /** Answers a cancellation; takes the invitation out of [pending] by default. */
    var revokeAnswer: suspend (invitationId: String) -> Unit = { id ->
        this.pending =
            this.pending.filter { it.id != id }
    }

    private var sentCount = 0
    private val inviteCalls = mutableListOf<Pair<String, Role>>()
    private val revokeCalls = mutableListOf<String>()

    /** The (email, role) invitations asked for, in order. */
    val invites: List<Pair<String, Role>> get() = inviteCalls.toList()

    /** The ids of every invitation asked to be cancelled, in order. */
    val revocations: List<String> get() = revokeCalls.toList()

    override suspend fun page(
        cursor: String?,
        limit: Int,
    ): Page<Invitation> = pageAnswer(cursor)

    override suspend fun invite(
        email: String,
        role: Role,
    ): Invitation {
        inviteCalls += email to role
        return inviteAnswer(email, role)
    }

    override suspend fun revoke(invitationId: String) {
        revokeCalls += invitationId
        revokeAnswer(invitationId)
    }

    private companion object {
        /** The expiry a new invitation gets: a week after the tests' "now". */
        val NEXT_WEEK = Instant.parse("2026-12-02T12:00:00Z")
    }
}
