package br.com.encomendo.core.domain.members

import br.com.encomendo.core.domain.session.Role
import kotlin.time.Instant

/**
 * Invitation is a pending invitation to the store's team (`Invitation` in
 * docs/api/openapi.yaml, ADR 0011): not yet accepted, revoked or expired.
 * [toString] prints only the id, so an invitation in a log carries no email.
 */
data class Invitation(
    /** The invitation's id (a UUID). Sending it again can give it a new one. */
    val id: String,
    /** The email it went to. */
    val email: String,
    /** The role the person gets on accepting; never the owner's. */
    val role: Role,
    /** When its link stops working. */
    val expiresAt: Instant,
) {
    /**
     * Names the invitation's row: its email, compared without regard to case
     * as the API does. Sending again replaces the invitation, so its id can
     * change while the row stays.
     */
    val key: String get() = email.lowercase()

    override fun toString(): String = "Invitation(id=$id, role=${role.key})"
}
