package br.com.encomendo.feature.members

import br.com.encomendo.core.domain.members.Invitation
import br.com.encomendo.core.domain.members.Member
import br.com.encomendo.core.domain.session.Role
import br.com.encomendo.core.ui.failure.FailureMessage

/**
 * MembersUiState is the Equipe screen: the team, the roles drafted but not
 * saved, the question a change asks first, the invite form, the pending
 * invitations, and what the last read or change said. Members and
 * invitations print only their ids, and the form leaves out what was typed,
 * so the state carries no personal data into a log.
 */
data class MembersUiState(
    /** The team, oldest member first, as far as it was read. */
    val members: List<Member> = emptyList(),
    /** The signed-in member's id, whose row says "(você)". */
    val signedInUserId: String? = null,
    /** The team's first page is being read: "Carregando a equipe…" while nothing shows yet. */
    val loading: Boolean = true,
    /** The team runs past what was read: "Carregar mais". */
    val hasMore: Boolean = false,
    /** The team's next page is being read. */
    val loadingMore: Boolean = false,
    /** Roles picked but not saved, by userId; a row shows "Alterar papel" while it has one. */
    val drafts: Map<String, Role> = emptyMap(),
    /** The question a change asks before it is made, or null. */
    val question: Question? = null,
    /** "Convidar para a equipe". */
    val invite: InviteFormUi = InviteFormUi(),
    /** "Convites pendentes", newest first, as far as they were read. */
    val invitations: List<Invitation> = emptyList(),
    /** The invitations run past what was read: "Carregar mais convites". */
    val invitationsHaveMore: Boolean = false,
    /** The invitations' next page is being read. */
    val loadingMoreInvitations: Boolean = false,
    /** The row whose change is being saved; every row's buttons wait meanwhile. */
    val saving: RowKey? = null,
    /** The last change that went through. */
    val notice: TeamNotice? = null,
    /** Why a list could not be read, or why a change to a row now gone failed: shown at the top. */
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

    /** Why the last change to [row] failed, if it did. */
    fun errorIn(row: RowKey): TeamMessage? = rowError?.takeIf { it.row == row }?.message
}

/**
 * InviteFormUi is "Convidar para a equipe": an email and a role, Funcionário
 * unless another is chosen. [toString] leaves out the email.
 */
data class InviteFormUi(
    val email: String = "",
    val role: Role = Role.Staff,
    /** What is wrong with the email, shown under it. */
    val emailIssue: EmailIssue? = null,
    /** Why the last invitation failed, shown above the form. */
    val error: TeamMessage? = null,
    /** An invitation is being sent; "Enviar convite" waits. */
    val sending: Boolean = false,
    /**
     * Counts the refusals about the email. Each new one takes the keyboard to
     * the field once, as on the web; the screen remembers the last it served.
     */
    val emailFocusRequests: Int = 0,
) {
    override fun toString(): String =
        "InviteFormUi(role=${role.key}, emailIssue=${emailIssue != null}, error=$error, sending=$sending)"
}

/** EmailIssue is what is wrong with the invite form's email. */
sealed interface EmailIssue {
    /** "Informe o e-mail." */
    data object Missing : EmailIssue

    /** "Esta pessoa já faz parte da equipe." */
    data object AlreadyOnTeam : EmailIssue

    /** Refused by the server, in its own pt-BR words. */
    data class Server(
        val message: String,
    ) : EmailIssue {
        // The server's words may quote what was typed.
        override fun toString(): String = "Server"
    }
}

/** RowKey names one row of Equipe: a member's, or a pending invitation's. */
sealed interface RowKey {
    /** The row of the member [userId]. */
    data class OfMember(
        val userId: String,
    ) : RowKey

    /** The row of the invitation whose [Invitation.key] is [key]; a new link keeps the row. */
    data class OfInvitation(
        val key: String,
    ) : RowKey {
        // The key is an email.
        override fun toString(): String = "OfInvitation"
    }
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

    /** "Convite enviado para {e-mail}." */
    data class InvitationSent(
        val invitation: Invitation,
    ) : TeamNotice

    /** "Convite reenviado para {e-mail}." */
    data class InvitationResent(
        val invitation: Invitation,
    ) : TeamNotice

    /** "Convite para {e-mail} cancelado." */
    data class InvitationRevoked(
        val invitation: Invitation,
    ) : TeamNotice
}

/** TeamMessage is why a read or a change failed, in the design document's words for Equipe. */
sealed interface TeamMessage {
    /** "Você não tem mais permissão para gerenciar a equipe."; the screen then leaves for Hoje. */
    data object LostPermission : TeamMessage

    /** "Esta pessoa não faz mais parte da equipe.", shown at the top while the lists are read again. */
    data object NotOnTeam : TeamMessage

    /** "Este convite não está mais pendente.", shown at the top while the lists are read again. */
    data object NotPending : TeamMessage

    /**
     * "Esta pessoa já faz parte da equipe.": they joined meanwhile. Shown at
     * the top while the lists are read again.
     */
    data object AlreadyOnTeam : TeamMessage

    /** 'O convite foi salvo, mas o e-mail não saiu. Use "Reenviar" para tentar de novo.' */
    data object NotSent : TeamMessage

    /** "Confira os campos destacados.", above the invite form. */
    data object CheckFields : TeamMessage

    /** Any other failure, in the words every staff screen uses. */
    data class General(
        val message: FailureMessage,
    ) : TeamMessage
}

/** RowError is why the last change to [row] failed. */
data class RowError(
    val row: RowKey,
    val message: TeamMessage,
)
