package br.com.encomendo.core.security

import br.com.encomendo.core.domain.session.GrantedPermissions
import br.com.encomendo.core.domain.session.Membership
import br.com.encomendo.core.domain.session.Permission
import br.com.encomendo.core.domain.session.Role
import br.com.encomendo.core.domain.session.Session
import br.com.encomendo.core.domain.session.TenantSummary
import br.com.encomendo.core.domain.session.User
import kotlinx.serialization.Serializable

/**
 * SessionRecord is the JSON shape of a [StoredSession] inside the sealed blob.
 * It is a storage format, not the API's: renaming a field here would sign
 * every device out, so add fields with defaults instead.
 *
 * The domain session is mapped here rather than in core:data (ADR 0015) because it is
 * sealed in the same blob as the tokens: one write keeps them in step, so the
 * permissions on screen always belong to the tokens in use. The API's shape
 * is mapped in core:data, as usual.
 */
@Serializable
internal class SessionRecord(
    val access: String,
    val refresh: String,
    val user: UserRecord,
    val tenant: TenantRecord? = null,
    val role: String? = null,
    val permissions: List<String> = emptyList(),
    val memberships: List<MembershipRecord> = emptyList(),
)

@Serializable
internal class UserRecord(
    val id: String,
    val name: String,
    val email: String,
)

@Serializable
internal class TenantRecord(
    val id: String,
    val name: String,
    val slug: String,
)

@Serializable
internal class MembershipRecord(
    val tenant: TenantRecord,
    val role: String,
    // Added with ADR 0022; a session stored before it has no stores suspended.
    val suspended: Boolean = false,
)

internal fun StoredSession.toRecord(): SessionRecord =
    SessionRecord(
        access = tokens.accessToken,
        refresh = tokens.refreshToken,
        user = session.user.let { UserRecord(id = it.id, name = it.name, email = it.email) },
        tenant = session.tenant?.toRecord(),
        role = session.role?.key,
        permissions = session.permissions.keys.map { it.key },
        memberships = session.memberships.map { MembershipRecord(it.tenant.toRecord(), it.role.key, it.suspended) },
    )

internal fun SessionRecord.toStoredSession(): StoredSession =
    StoredSession(
        tokens = SessionTokens(accessToken = access, refreshToken = refresh),
        session =
            Session(
                user = User(id = user.id, name = user.name, email = user.email),
                tenant = tenant?.toTenant(),
                role = role?.let(::Role),
                permissions = GrantedPermissions(permissions.map(::Permission)),
                memberships = memberships.map { Membership(it.tenant.toTenant(), Role(it.role), it.suspended) },
            ),
    )

private fun TenantSummary.toRecord(): TenantRecord = TenantRecord(id = id, name = name, slug = slug)

private fun TenantRecord.toTenant(): TenantSummary = TenantSummary(id = id, name = name, slug = slug)
