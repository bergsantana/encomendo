package br.com.encomendo.core.data.members

import br.com.encomendo.core.data.failure.StoreCalls
import br.com.encomendo.core.domain.members.Member
import br.com.encomendo.core.domain.members.MembersRepository
import br.com.encomendo.core.domain.paging.Page
import br.com.encomendo.core.domain.session.Role
import br.com.encomendo.core.network.members.MemberDto
import br.com.encomendo.core.network.members.MembersApi

/** ApiMembersRepository reads and changes the team through [api], in the session's store. */
class ApiMembersRepository(
    private val api: MembersApi,
    private val calls: StoreCalls,
) : MembersRepository {
    override suspend fun page(
        cursor: String?,
        limit: Int,
    ): Page<Member> =
        calls.inStore {
            val page = api.list(cursor = cursor, limit = limit)
            Page(items = page.items.map { it.toMember() }, nextCursor = page.nextCursor)
        }

    override suspend fun changeRole(
        userId: String,
        role: Role,
    ): Member = calls.inStore { api.changeRole(userId = userId, role = role.key).toMember() }

    override suspend fun remove(userId: String) = calls.inStore { api.remove(userId = userId) }
}

private fun MemberDto.toMember(): Member = Member(userId = userId, name = name, email = email, role = Role(role))
