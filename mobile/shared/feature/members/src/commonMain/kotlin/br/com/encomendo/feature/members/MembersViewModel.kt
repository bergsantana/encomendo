package br.com.encomendo.feature.members

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.domain.members.ChangeMemberRole
import br.com.encomendo.core.domain.members.Invitation
import br.com.encomendo.core.domain.members.InviteMember
import br.com.encomendo.core.domain.members.ListInvitations
import br.com.encomendo.core.domain.members.ListMembers
import br.com.encomendo.core.domain.members.Member
import br.com.encomendo.core.domain.members.RemoveMember
import br.com.encomendo.core.domain.members.RevokeInvitation
import br.com.encomendo.core.domain.session.ObserveSession
import br.com.encomendo.core.domain.session.Role
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/** MembersUseCases are what Equipe does through the API, so [MembersViewModel] takes them as one input. */
class MembersUseCases(
    val listMembers: ListMembers,
    val changeMemberRole: ChangeMemberRole,
    val removeMember: RemoveMember,
    val listInvitations: ListInvitations,
    val inviteMember: InviteMember,
    val revokeInvitation: RevokeInvitation,
)

/**
 * MembersViewModel backs Equipe, as the web's single members store: the team
 * and its roles, removals, the invite form and the pending invitations
 * (docs/product/design-document.md, "Equipe"). It asks first where the
 * design document says so, and runs one change to a row at a time. The two
 * lists share their messages: a row that is no longer what the screen shows
 * says so at the top, and both lists are read again. The use cases renew the
 * session after a change to one's own row or a refusal; the app then leaves
 * Equipe if its permission went.
 */
@Suppress("TooManyFunctions") // One function per thing the user can do on Equipe, as the web's members store.
class MembersViewModel(
    observeSession: ObserveSession,
    private val useCases: MembersUseCases,
) : ViewModel() {
    private val team = MutableStateFlow(MembersUiState())
    private val membersPager = Pager(viewModelScope) { cursor -> useCases.listMembers(cursor) }
    private val invitationsPager = Pager(viewModelScope) { cursor -> useCases.listInvitations(cursor) }

    private val nextMembers =
        PageReader<Member>(
            onStart = { team.update { it.clearingMessages().copy(loadingMore = true) } },
            onPage = { page -> team.update { it.showingNextPage(page) } },
            onFailure = { failure -> team.update { it.copy(error = failure.toTeamMessage()) } },
            onEnd = { team.update { it.copy(loadingMore = false) } },
        )

    private val nextInvitations =
        PageReader<Invitation>(
            onStart = { team.update { it.clearingMessages().copy(loadingMoreInvitations = true) } },
            onPage = { page -> team.update { it.showingNextInvitations(page) } },
            onFailure = { failure -> team.update { it.copy(error = failure.toTeamMessage()) } },
            onEnd = { team.update { it.copy(loadingMoreInvitations = false) } },
        )

    /** The screen as it shows now. */
    val uiState: StateFlow<MembersUiState> = team.asStateFlow()

    init {
        // Kept in the state itself, not only in what the screen sees: "Alterar papel" asks first on one's own row.
        viewModelScope.launch {
            observeSession().collect { session -> team.update { it.copy(signedInUserId = session?.user?.id) } }
        }
        readLists()
    }

    /** "Tentar de novo" after the team could not be read. */
    fun onRetry() {
        if (!team.value.loading) readLists()
    }

    /** "Carregar mais": the team's next page. Problems show at the top. */
    fun onLoadMore() {
        membersPager.readNext(nextMembers)
    }

    /** "Carregar mais convites": the invitations' next page. Problems show at the top. */
    fun onLoadMoreInvitations() {
        invitationsPager.readNext(nextInvitations)
    }

    /**
     * The user picked [role] for [member]. It is only a draft until "Alterar
     * papel" (WCAG 3.2.2). Ignored while a change is saved or the row asks
     * its question, so the answer is about the role the question showed.
     */
    fun onPickRole(
        member: Member,
        role: Role,
    ) {
        if (!team.value.mayDraft(member) || role !in Role.Invitable) return
        team.update {
            val drafts = if (role == member.role) it.drafts - member.userId else it.drafts + (member.userId to role)
            it.copy(drafts = drafts)
        }
    }

    /** "Alterar papel": saves [member]'s draft; on one's own row it asks first. */
    fun onSaveRole(member: Member) {
        val state = team.value
        val role = state.drafts[member.userId]?.takeIf { state.saving == null } ?: return
        if (state.isMe(member)) {
            team.update { it.copy(question = Question(member.userId, QuestionKind.OwnRole)) }
        } else {
            change(member, role)
        }
    }

    /** "Remover": asks first, whoever it is. */
    fun onAskRemove(member: Member) {
        if (!member.canChange || team.value.saving != null) return
        team.update { it.copy(question = Question(member.userId, QuestionKind.Removal)) }
    }

    /** "Cancelar": puts things back as they were; a role question restores the role the member has. */
    fun onCancel() {
        team.update { state ->
            val question = state.question ?: return@update state
            val drafts = if (question.kind == QuestionKind.OwnRole) state.drafts - question.userId else state.drafts
            state.copy(question = null, drafts = drafts)
        }
    }

    /** "Confirmar" or "Confirmar remoção": makes the change the question was about. */
    fun onConfirm() {
        val state = team.value
        val question = state.question?.takeIf { state.saving == null } ?: return
        team.update { it.copy(question = null) }
        val member = state.members.firstOrNull { it.userId == question.userId } ?: return
        when (question.kind) {
            QuestionKind.OwnRole -> state.drafts[member.userId]?.let { change(member, it) }
            QuestionKind.Removal -> remove(member)
        }
    }

    /** The invite form's email changed; its marks go, but not a rate limit's wait. */
    fun onInviteEmailChange(email: String) {
        team.update { it.copy(invite = it.invite.typed(email)) }
    }

    /** The invite form's role changed: any role but the owner's. */
    fun onInviteRoleChange(role: Role) {
        if (role in Role.Invitable) team.update { it.copy(invite = it.invite.copy(role = role)) }
    }

    /** "Enviar convite". An empty email is marked, never sent. */
    fun onSendInvitation() {
        val form = team.value.invite.takeUnless { it.sending } ?: return
        if (form.email.isBlank()) {
            team.update {
                it.copy(
                    invite =
                        form
                            .copy(
                                emailIssue = EmailIssue.Missing,
                                error = TeamMessage.CheckFields,
                            ).focusingEmail(),
                )
            }
            return
        }
        team.update { it.clearingMessages().copy(invite = form.copy(emailIssue = null, error = null, sending = true)) }
        viewModelScope.launch {
            try {
                val sent = useCases.inviteMember(email = form.email, role = form.role)
                team.update { state ->
                    val invite = state.invite.sent(sentTo = form.email)
                    state.listing(sent).copy(invite = invite, notice = TeamNotice.InvitationSent(sent))
                }
            } catch (failure: Failure) {
                team.update { it.copy(invite = it.invite.refused(failure)) }
                // The invitation is saved even though its email did not go out: list it.
                if (failure is Failure.EmailNotSent) readInvitations()
            } finally {
                team.update { it.copy(invite = it.invite.copy(sending = false)) }
            }
        }
    }

    /** "Reenviar": the invitation again, with a new link and expiry. */
    fun onResend(invitation: Invitation) {
        if (team.value.saving != null) return
        saving(RowKey.OfInvitation(invitation.key)) {
            val renewed = useCases.inviteMember(email = invitation.email, role = invitation.role)
            team.update { it.listing(renewed).copy(notice = TeamNotice.InvitationResent(renewed)) }
        }
    }

    /** "Cancelar convite": its link stops working. */
    fun onRevoke(invitation: Invitation) {
        if (team.value.saving != null) return
        saving(RowKey.OfInvitation(invitation.key)) {
            useCases.revokeInvitation(invitation)
            team.update { state ->
                state.copy(
                    invitations = state.invitations.filter { it.id != invitation.id },
                    notice = TeamNotice.InvitationRevoked(invitation),
                )
            }
        }
    }

    private fun change(
        member: Member,
        role: Role,
    ) = saving(RowKey.OfMember(member.userId)) {
        val changed = useCases.changeMemberRole(member, role)
        team.update { state ->
            state.copy(
                members = state.members.map { if (it.userId == changed.userId) changed else it },
                drafts = state.drafts - member.userId,
                notice = TeamNotice.RoleChanged(changed),
            )
        }
    }

    private fun remove(member: Member) =
        saving(RowKey.OfMember(member.userId)) {
            useCases.removeMember(member)
            team.update { state ->
                state.copy(
                    members = state.members.filter { it.userId != member.userId },
                    drafts = state.drafts - member.userId,
                    notice = TeamNotice.Removed(member),
                )
            }
        }

    /**
     * Runs one [change] to [row]. A failure shows in the row. A row that is
     * no longer what the screen shows (gone, or the person joined) says so at
     * the top instead, since it changes when the lists are read again.
     */
    private fun saving(
        row: RowKey,
        change: suspend () -> Unit,
    ) {
        team.update { it.clearingMessages().copy(saving = row) }
        viewModelScope.launch {
            try {
                change()
            } catch (failure: Failure) {
                val stale = failure.staleIn(row)
                team.update { state ->
                    if (stale !=
                        null
                    ) {
                        state.copy(error = stale)
                    } else {
                        state.copy(rowError = RowError(row, failure.toTeamMessage()))
                    }
                }
                // An invitation sent again without its email still has a new link and expiry.
                if (stale != null || failure is Failure.EmailNotSent) readLists(keepError = true)
            } finally {
                team.update { it.copy(saving = null) }
            }
        }
    }

    /**
     * Reads both lists from the start. [keepError] keeps the message at the
     * top, which a failed re-read must not hide.
     */
    private fun readLists(keepError: Boolean = false) {
        team.update { it.copy(loading = true, loadingMore = false, error = it.error.takeIf { keepError }) }
        membersPager.readFirst(firstMembers(keepError))
        readInvitations(keepError)
    }

    private fun readInvitations(keepError: Boolean = true) {
        team.update { it.copy(loadingMoreInvitations = false) }
        invitationsPager.readFirst(
            PageReader(
                onPage = { page -> team.update { it.showingFirstInvitations(page) } },
                onFailure = { failure -> team.update { it.failedToRead(failure, keepError) } },
                onEnd = {},
            ),
        )
    }

    private fun firstMembers(keepError: Boolean) =
        PageReader<Member>(
            onPage = { page -> team.update { it.showingFirstPage(page) } },
            onFailure = { failure -> team.update { it.failedToRead(failure, keepError) } },
            onEnd = { team.update { it.copy(loading = false) } },
        )
}
