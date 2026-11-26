package br.com.encomendo.core.domain.members

import br.com.encomendo.core.domain.session.Role

/**
 * Member is one person on the store's team (Equipe), with their role there
 * (`Member` in docs/api/openapi.yaml). [toString] prints only the id, so a
 * member in a log line or a failed test carries no personal data.
 */
data class Member(
    /** The person's account id (a UUID); the API names members by it. */
    val userId: String,
    /** The name they signed up with. */
    val name: String,
    /** Their login email. */
    val email: String,
    /** Their role in this store. */
    val role: Role,
) {
    /**
     * False for the owner: the owner's role never changes and the owner is
     * never removed (ADR 0011). The API refuses both anyway.
     */
    val canChange: Boolean get() = role != Role.Owner

    override fun toString(): String = "Member(userId=$userId, role=${role.key})"
}

/** MemberPage is one page of the team, oldest member first; [nextCursor] is null on the last page. */
data class MemberPage(
    /** The members on this page. */
    val members: List<Member>,
    /** Where the next page starts, or null when this is the last. */
    val nextCursor: String?,
)
