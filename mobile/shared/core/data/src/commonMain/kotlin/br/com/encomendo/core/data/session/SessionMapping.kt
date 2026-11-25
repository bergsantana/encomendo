package br.com.encomendo.core.data.session

import br.com.encomendo.core.domain.session.GrantedPermissions
import br.com.encomendo.core.domain.session.Membership
import br.com.encomendo.core.domain.session.Permission
import br.com.encomendo.core.domain.session.Role
import br.com.encomendo.core.domain.session.Session
import br.com.encomendo.core.domain.session.TenantSummary
import br.com.encomendo.core.domain.session.User
import br.com.encomendo.core.network.auth.AuthSessionDto
import br.com.encomendo.core.network.auth.TenantSummaryDto
import br.com.encomendo.core.security.SessionTokens
import br.com.encomendo.core.security.StoredSession

/** Maps an API session to what the device stores: the tokens and the session they belong to. */
internal fun AuthSessionDto.toStoredSession(): StoredSession =
    StoredSession(
        tokens = SessionTokens(accessToken = accessToken, refreshToken = refreshToken),
        session =
            Session(
                user = User(id = user.id, name = user.name, email = user.email),
                tenant = tenant?.toTenant(),
                role = role?.let(::Role),
                permissions = GrantedPermissions(permissions.map(::Permission)),
                memberships = memberships.map { Membership(it.tenant.toTenant(), Role(it.role)) },
            ),
    )

private fun TenantSummaryDto.toTenant(): TenantSummary = TenantSummary(id = id, name = name, slug = slug)
