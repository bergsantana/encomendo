package br.com.encomendo.feature.members

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.domain.members.ChangeMemberRole
import br.com.encomendo.core.domain.members.ListMembers
import br.com.encomendo.core.domain.members.Member
import br.com.encomendo.core.domain.members.MemberPage
import br.com.encomendo.core.domain.members.RemoveMember
import br.com.encomendo.core.domain.session.ObserveSession
import br.com.encomendo.core.domain.session.Role
import br.com.encomendo.core.ui.failure.toFailureMessage
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/**
 * MembersViewModel backs Equipe: it reads the team, drafts and saves roles,
 * and removes members, asking first where the design document says so. One
 * change runs at a time. The use cases renew the session after a change to
 * one's own row or a refusal; the app then leaves Equipe if its permission
 * went (docs/product/design-document.md, "Equipe").
 */
class MembersViewModel(
    observeSession: ObserveSession,
    private val listMembers: ListMembers,
    private val changeMemberRole: ChangeMemberRole,
    private val removeMember: RemoveMember,
) : ViewModel() {
    private val team = MutableStateFlow(MembersUiState())

    // Where the next page starts, and the reads under way; read and replaced on the main thread only.
    private var nextCursor: String? = null
    private var reading: Job? = null
    private var readCount = 0
    private var readingMore: Job? = null

    /** The screen as it shows now. */
    val uiState: StateFlow<MembersUiState> = team.asStateFlow()

    init {
        // Kept in the state itself, not only in what the screen sees: "Alterar papel" asks first on one's own row.
        viewModelScope.launch {
            observeSession().collect { session -> team.update { it.copy(signedInUserId = session?.user?.id) } }
        }
        readFirstPage()
    }

    /** "Tentar de novo" after the team could not be read. */
    fun onRetry() {
        if (!team.value.loading) readFirstPage()
    }

    /** "Carregar mais": the next page of the team. Problems show at the top. */
    fun onLoadMore() {
        val cursor = nextCursor.takeUnless { team.value.loading || team.value.loadingMore } ?: return
        team.update { it.clearingMessages().copy(loadingMore = true) }
        readingMore =
            viewModelScope.launch {
                try {
                    val page = listMembers(cursor)
                    nextCursor = page.nextCursor
                    team.update { it.showingNextPage(page) }
                } catch (failure: Failure) {
                    team.update { it.copy(error = failure.toTeamMessage()) }
                } finally {
                    team.update { it.copy(loadingMore = false) }
                }
            }
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
        val role = state.drafts[member.userId]?.takeIf { state.savingUserId == null } ?: return
        if (state.isMe(member)) {
            team.update { it.copy(question = Question(member.userId, QuestionKind.OwnRole)) }
        } else {
            change(member, role)
        }
    }

    /** "Remover": asks first, whoever it is. */
    fun onAskRemove(member: Member) {
        if (!member.canChange || team.value.savingUserId != null) return
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
        val question = state.question?.takeIf { state.savingUserId == null } ?: return
        team.update { it.copy(question = null) }
        val member = state.members.firstOrNull { it.userId == question.userId } ?: return
        when (question.kind) {
            QuestionKind.OwnRole -> state.drafts[member.userId]?.let { change(member, it) }
            QuestionKind.Removal -> remove(member)
        }
    }

    private fun change(
        member: Member,
        role: Role,
    ) = saving(member) {
        val changed = changeMemberRole(member, role)
        team.update { state ->
            state.copy(
                members = state.members.map { if (it.userId == changed.userId) changed else it },
                drafts = state.drafts - member.userId,
                notice = TeamNotice.RoleChanged(changed),
            )
        }
    }

    private fun remove(member: Member) =
        saving(member) {
            removeMember(member)
            team.update { state ->
                state.copy(
                    members = state.members.filter { it.userId != member.userId },
                    drafts = state.drafts - member.userId,
                    notice = TeamNotice.Removed(member),
                )
            }
        }

    /**
     * Runs one [change] to [member]'s row. A failure shows in the row; a row
     * gone meanwhile is said at the top instead, and the team is read again.
     */
    private fun saving(
        member: Member,
        change: suspend () -> Unit,
    ) {
        team.update { it.clearingMessages().copy(savingUserId = member.userId) }
        viewModelScope.launch {
            try {
                change()
            } catch (_: Failure.NotFound) {
                team.update { it.copy(error = TeamMessage.NotOnTeam) }
                readFirstPage(keepError = true)
            } catch (failure: Failure) {
                team.update { it.copy(rowError = RowError(member.userId, failure.toTeamMessage())) }
            } finally {
                team.update { it.copy(savingUserId = null) }
            }
        }
    }

    /**
     * Reads the team from the start. [keepError] keeps the message at the
     * top, which a failed re-read must not hide.
     */
    private fun readFirstPage(keepError: Boolean = false) {
        // A page asked for before this read would follow a list that is no longer on screen.
        reading?.cancel()
        readingMore?.cancel()
        // Only the latest read says it is done: a cancelled one may finish after its replacement started.
        val read = ++readCount
        team.update { it.copy(loading = true, error = it.error.takeIf { keepError }) }
        reading =
            viewModelScope.launch {
                try {
                    val page = listMembers()
                    nextCursor = page.nextCursor
                    team.update { it.showingFirstPage(page) }
                } catch (failure: Failure) {
                    team.update { it.copy(error = if (keepError) it.error else failure.toTeamMessage()) }
                } finally {
                    if (read == readCount) team.update { it.copy(loading = false) }
                }
            }
    }
}

/** True when [member]'s role may be drafted: not the owner, no change being saved, no question open on the row. */
private fun MembersUiState.mayDraft(member: Member): Boolean =
    member.canChange && savingUserId == null && question?.userId != member.userId

/** The next page added after the rows on screen; a member already listed is not listed twice. */
private fun MembersUiState.showingNextPage(page: MemberPage): MembersUiState {
    val listed = members.mapTo(HashSet()) { it.userId }
    return copy(members = members + page.members.filter { it.userId !in listed }, hasMore = page.nextCursor != null)
}

/**
 * The team read again from its first page. A draft stays only for a row
 * still there with the role it had, as the web's row keeps its draft until
 * a saved or re-read role replaces it. A question goes with its row, and a
 * question about one's own role goes with its draft.
 */
private fun MembersUiState.showingFirstPage(page: MemberPage): MembersUiState {
    val before = members.associateBy { it.userId }
    val kept =
        drafts.filter { (userId, _) ->
            val now = page.members.firstOrNull { it.userId == userId }
            now != null && now.role == before[userId]?.role
        }
    return copy(
        members = page.members,
        hasMore = page.nextCursor != null,
        drafts = kept,
        question =
            question?.takeIf { asked ->
                page.members.any { it.userId == asked.userId } &&
                    (asked.kind == QuestionKind.Removal || asked.userId in kept)
            },
    )
}

/** Each change starts afresh: the last change's notice and errors go. */
private fun MembersUiState.clearingMessages(): MembersUiState = copy(notice = null, error = null, rowError = null)

/** Equipe's words for [this] failure: its own for a lost permission and a member gone, every screen's otherwise. */
internal fun Failure.toTeamMessage(): TeamMessage =
    when (this) {
        is Failure.Forbidden -> TeamMessage.LostPermission
        is Failure.NotFound -> TeamMessage.NotOnTeam
        else -> TeamMessage.General(toFailureMessage())
    }
