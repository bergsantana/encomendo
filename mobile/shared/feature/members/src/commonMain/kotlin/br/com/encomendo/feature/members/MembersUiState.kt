package br.com.encomendo.feature.members

import br.com.encomendo.core.domain.members.Member
import br.com.encomendo.core.domain.session.Role
import br.com.encomendo.core.ui.failure.FailureMessage

/**
 * MembersUiState is the Equipe screen: the team, the roles drafted but not
 * saved, the question a change asks first, and what the last read or
 * change said. Members print only their ids, so the state carries no
 * personal data into a log.
 */
data class MembersUiState(
    /** The team, oldest member first, as far as it was read. */
    val members: List<Member> = emptyList(),
    /** The signed-in member's id, whose row says "(você)". */
    val signedInUserId: String? = null,
    /** The first page is being read: "Carregando a equipe…" while nothing shows yet. */
    val loading: Boolean = true,
    /** The team runs past what was read: "Carregar mais". */
    val hasMore: Boolean = false,
    /** The next page is being read. */
    val loadingMore: Boolean = false,
    /** Roles picked but not saved, by userId; a row shows "Alterar papel" while it has one. */
    val drafts: Map<String, Role> = emptyMap(),
    /** The question a change asks before it is made, or null. */
    val question: Question? = null,
    /** The member whose change is being saved; every row's buttons wait meanwhile. */
    val savingUserId: String? = null,
    /** The last change that went through. */
    val notice: TeamNotice? = null,
    /** Why the team could not be read, or why a change to a row now gone failed: shown at the top. */
    val error: TeamMessage? = null,
    /** Why the last change to a row still on screen failed: shown in that row. */
    val rowError: RowError? = null,
) {
    /** The team could not be read at all: the message and "Tentar de novo". */
    val canRetry: Boolean get() = error != null && members.isEmpty() && !loading

    /** True on the signed-in member's own row. */
    fun isMe(member: Member): Boolean = member.userId == signedInUserId

    /** The role [member]'s choice shows: the draft, or the role they have. */
    fun draftFor(member: Member): Role = drafts[member.userId] ?: member.role

    /** True once a role other than [member]'s own is picked for them, so "Alterar papel" shows. */
    fun hasDraft(member: Member): Boolean = member.userId in drafts
}

/** Question is what a change asks first on the row of [userId]. */
data class Question(
    val userId: String,
    val kind: QuestionKind,
)

/** QuestionKind is which change a [Question] is about. */
enum class QuestionKind {
    /** One's own role: "Você deixará de gerenciar a equipe." */
    OwnRole,

    /** A removal: "Remover {nome} da equipe?", or for oneself "Você sairá da equipe e perderá o acesso a esta loja." */
    Removal,
}

/** TeamNotice is a change that went through, for the status line. */
sealed interface TeamNotice {
    /** "{nome} agora é {papel}." with [member] as saved. */
    data class RoleChanged(
        val member: Member,
    ) : TeamNotice

    /** "{nome} saiu da equipe." */
    data class Removed(
        val member: Member,
    ) : TeamNotice
}

/** TeamMessage is why a read or a change failed, in the design document's words for Equipe. */
sealed interface TeamMessage {
    /** "Você não tem mais permissão para gerenciar a equipe."; the screen then leaves for Hoje. */
    data object LostPermission : TeamMessage

    /** "Esta pessoa não faz mais parte da equipe.", shown at the top while the team is read again. */
    data object NotOnTeam : TeamMessage

    /** Any other failure, in the words every staff screen uses. */
    data class General(
        val message: FailureMessage,
    ) : TeamMessage
}

/** RowError is why the last change to [userId]'s row failed. */
data class RowError(
    val userId: String,
    val message: TeamMessage,
)
