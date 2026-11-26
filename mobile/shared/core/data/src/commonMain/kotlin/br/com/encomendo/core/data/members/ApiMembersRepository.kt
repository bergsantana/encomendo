package br.com.encomendo.core.data.members

import br.com.encomendo.core.data.failure.asFailures
import br.com.encomendo.core.domain.members.Member
import br.com.encomendo.core.domain.members.MemberPage
import br.com.encomendo.core.domain.members.MembersRepository
import br.com.encomendo.core.domain.session.Role
import br.com.encomendo.core.network.members.MemberDto
import br.com.encomendo.core.network.members.MembersApi

/** ApiMembersRepository reads and changes the team through [api], in the session's store. */
class ApiMembersRepository(
    private val api: MembersApi,
) : MembersRepository {
    override suspend fun page(
        cursor: String?,
        limit: Int,
    ): MemberPage =
        asFailures {
            val page = api.list(cursor = cursor, limit = limit)
            MemberPage(members = page.items.map { it.toMember() }, nextCursor = page.nextCursor)
        }

    override suspend fun changeRole(
        userId: String,
        role: Role,
    ): Member = asFailures { api.changeRole(userId = userId, role = role.key).toMember() }

    override suspend fun remove(userId: String) = asFailures { api.remove(userId = userId) }
}

private fun MemberDto.toMember(): Member = Member(userId = userId, name = name, email = email, role = Role(role))
