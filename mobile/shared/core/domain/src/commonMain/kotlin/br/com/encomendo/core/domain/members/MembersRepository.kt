package br.com.encomendo.core.domain.members

import br.com.encomendo.core.domain.paging.Page
import br.com.encomendo.core.domain.session.Role

/**
 * MembersRepository reads and changes the selected store's team. Every call
 * needs `members:manage`, which the API checks against the caller's current
 * role, not the one in their token (ADR 0011); without it, it throws
 * `Failure.Forbidden`.
 */
interface MembersRepository {
    /** Returns up to [limit] members from [cursor] on (null for the first page). */
    suspend fun page(
        cursor: String?,
        limit: Int,
    ): Page<Member>

    /**
     * Gives [userId] the role [role] and returns the member as saved. Throws
     * `Failure.NotFound` when they are no longer on the team.
     */
    suspend fun changeRole(
        userId: String,
        role: Role,
    ): Member

    /**
     * Takes [userId] off the team; they keep their account. Throws
     * `Failure.NotFound` when they are no longer on the team.
     */
    suspend fun remove(userId: String)
}
