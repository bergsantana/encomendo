package br.com.encomendo.feature.members

import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.domain.members.Invitation
import br.com.encomendo.core.domain.members.Member
import br.com.encomendo.core.domain.paging.Page
import br.com.encomendo.core.ui.failure.toFailureMessage

// How Equipe's state changes with each answer, apart from MembersViewModel so each rule reads on its own.

/** True when [member]'s role may be drafted: not the owner, no change being saved, no question open on the row. */
internal fun MembersUiState.mayDraft(member: Member): Boolean =
    member.canChange && saving == null && question?.userId != member.userId

/**
 * The team read again from its first page. A draft stays only for a row
 * still there with the role it had, as the web's row keeps its draft until
 * a saved or re-read role replaces it. A question goes with its row, and a
 * question about one's own role goes with its draft.
 */
internal fun MembersUiState.showingFirstPage(page: Page<Member>): MembersUiState {
    val before = members.associateBy { it.userId }
    val kept =
        drafts.filter { (userId, _) ->
            val now = page.items.firstOrNull { it.userId == userId }
            now != null && now.role == before[userId]?.role
        }
    return copy(
        members = page.items,
        hasMore = page.nextCursor != null,
        drafts = kept,
        question =
            question?.takeIf { asked ->
                page.items.any { it.userId == asked.userId } &&
                    (asked.kind == QuestionKind.Removal || asked.userId in kept)
            },
    )
}

/** The team's next page after the rows on screen; a member already listed is not listed twice. */
internal fun MembersUiState.showingNextPage(page: Page<Member>): MembersUiState {
    val listed = members.mapTo(HashSet()) { it.userId }
    return copy(members = members + page.items.filter { it.userId !in listed }, hasMore = page.nextCursor != null)
}

/** The pending invitations read again from their first page. */
internal fun MembersUiState.showingFirstInvitations(page: Page<Invitation>): MembersUiState =
    copy(invitations = page.items, invitationsHaveMore = page.nextCursor != null)

/** The invitations' next page after the rows on screen; an invitation already listed is not listed twice. */
internal fun MembersUiState.showingNextInvitations(page: Page<Invitation>): MembersUiState {
    val listed = invitations.mapTo(HashSet()) { it.key }
    return copy(
        invitations = invitations + page.items.filter { it.key !in listed },
        invitationsHaveMore = page.nextCursor != null,
    )
}

/** [invitation] listed first, in place of a pending one for the same email: sending again replaces it. */
internal fun MembersUiState.listing(invitation: Invitation): MembersUiState =
    copy(invitations = listOf(invitation) + invitations.filter { it.key != invitation.key })

/** A list that could not be read says why at the top, unless [keepError] keeps the message already there. */
internal fun MembersUiState.failedToRead(
    failure: Failure,
    keepError: Boolean,
): MembersUiState = copy(error = if (keepError) error else failure.toTeamMessage())

/** Each change starts afresh: the last change's notice and errors go. */
internal fun MembersUiState.clearingMessages(): MembersUiState = copy(notice = null, error = null, rowError = null)

/**
 * The message at the top when a change to [row] finds the row no longer what
 * the screen shows: the member or the invitation is gone, or the person
 * joined meanwhile. Null when the failure belongs in the row.
 */
internal fun Failure.staleIn(row: RowKey): TeamMessage? =
    when (this) {
        is Failure.NotFound -> if (row is RowKey.OfMember) TeamMessage.NotOnTeam else TeamMessage.NotPending
        is Failure.AlreadyMember -> TeamMessage.AlreadyOnTeam
        else -> null
    }

/** Equipe's words for [this] failure: its own where the design document has them, every screen's otherwise. */
internal fun Failure.toTeamMessage(): TeamMessage =
    when (this) {
        is Failure.Forbidden -> TeamMessage.LostPermission
        is Failure.NotFound -> TeamMessage.NotOnTeam
        is Failure.AlreadyMember -> TeamMessage.AlreadyOnTeam
        is Failure.EmailNotSent -> TeamMessage.NotSent
        else -> TeamMessage.General(toFailureMessage())
    }
