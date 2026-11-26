package br.com.encomendo.core.domain.members

import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.domain.paging.Page
import br.com.encomendo.core.domain.session.Role
import br.com.encomendo.core.domain.session.SessionRepository
import kotlinx.coroutines.flow.first

/**
 * ListMembers reads the store's team a page at a time. A refusal renews the
 * session first, so a role lost elsewhere shows and Equipe can leave.
 */
class ListMembers(
    private val members: MembersRepository,
    private val sessions: SessionRepository,
) {
    /** Returns the page that starts at [cursor], or the first page. */
    suspend operator fun invoke(cursor: String? = null): Page<Member> =
        sessions.renewingWhenRefused { members.page(cursor = cursor, limit = PAGE_SIZE) }

    private companion object {
        /** Teams are small; one page is usually the whole team, as on the web. The contract allows 100 at most. */
        const val PAGE_SIZE = 100
    }
}

/**
 * ChangeMemberRole gives a member another role. Changing one's own role
 * renews the session, so the permissions shown follow at once, as the web's
 * `session.refresh()` does; so does a refusal.
 */
class ChangeMemberRole(
    private val members: MembersRepository,
    private val sessions: SessionRepository,
) {
    /** Returns [member] as saved with [role], which is never [Role.Owner]: a store has one owner. */
    suspend operator fun invoke(
        member: Member,
        role: Role,
    ): Member {
        require(role in Role.Invitable) { "only admin, staff or viewer can be given here" }
        val changed = sessions.renewingWhenRefused { members.changeRole(userId = member.userId, role = role) }
        sessions.renewIfSignedInAs(member)
        return changed
    }
}

/**
 * RemoveMember takes a member off the team; they keep their account.
 * Removing oneself renews the session, which then has no such store, so the
 * app leaves it (ADR 0010); so does a refusal.
 */
class RemoveMember(
    private val members: MembersRepository,
    private val sessions: SessionRepository,
) {
    /** Removes [member]. */
    suspend operator fun invoke(member: Member) {
        sessions.renewingWhenRefused { members.remove(userId = member.userId) }
        sessions.renewIfSignedInAs(member)
    }
}

/** Runs [call]; when the API refuses it, renews the session before failing, so the screen sees what it may do now. */
internal suspend fun <T> SessionRepository.renewingWhenRefused(call: suspend () -> T): T =
    try {
        call()
    } catch (refused: Failure.Forbidden) {
        renew()
        throw refused
    }

/** After a change to [member]'s own row, renews the session: the change was to the signed-in user's own access. */
private suspend fun SessionRepository.renewIfSignedInAs(member: Member) {
    if (session.first()?.user?.id == member.userId) renew()
}
