package br.com.encomendo.core.testing

import br.com.encomendo.core.domain.members.Member
import br.com.encomendo.core.domain.members.MemberPage
import br.com.encomendo.core.domain.members.MembersRepository
import br.com.encomendo.core.domain.session.Role

/**
 * FakeMembersRepository keeps a [team] in memory and records every call. By
 * default it answers as the API would: the whole team in one page, a role
 * saved, a member removed. Set [pageAnswer], [changeAnswer] or [removeAnswer]
 * to page differently, wait, or throw a `Failure`.
 */
class FakeMembersRepository(
    team: List<Member> = emptyList(),
) : MembersRepository {
    /** The team as the server holds it now; a test may change it to simulate someone else's change. */
    var team: List<Member> = team

    /** Answers a page request; the whole [team] by default. */
    var pageAnswer: suspend (cursor: String?) -> MemberPage = { MemberPage(members = this.team, nextCursor = null) }

    /** Answers a role change; saves it in [team] by default. */
    var changeAnswer: suspend (userId: String, role: Role) -> Member = { userId, role ->
        val changed = this.team.first { it.userId == userId }.copy(role = role)
        this.team = this.team.map { if (it.userId == userId) changed else it }
        changed
    }

    /** Answers a removal; takes the member out of [team] by default. */
    var removeAnswer: suspend (userId: String) -> Unit = { userId ->
        this.team =
            this.team.filter { it.userId != userId }
    }

    private val pageCalls = mutableListOf<String?>()
    private val changeCalls = mutableListOf<Pair<String, Role>>()
    private val removeCalls = mutableListOf<String>()

    /** The cursors of every page asked for, in order (null for a first page). */
    val pagesRead: List<String?> get() = pageCalls.toList()

    /** The (userId, role) changes asked for, in order. */
    val changes: List<Pair<String, Role>> get() = changeCalls.toList()

    /** The userIds whose removal was asked for, in order. */
    val removals: List<String> get() = removeCalls.toList()

    override suspend fun page(
        cursor: String?,
        limit: Int,
    ): MemberPage {
        pageCalls += cursor
        return pageAnswer(cursor)
    }

    override suspend fun changeRole(
        userId: String,
        role: Role,
    ): Member {
        changeCalls += userId to role
        return changeAnswer(userId, role)
    }

    override suspend fun remove(userId: String) {
        removeCalls += userId
        removeAnswer(userId)
    }
}
