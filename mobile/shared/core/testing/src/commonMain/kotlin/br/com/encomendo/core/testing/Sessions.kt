package br.com.encomendo.core.testing

import br.com.encomendo.core.domain.session.GrantedPermissions
import br.com.encomendo.core.domain.session.Membership
import br.com.encomendo.core.domain.session.Permission
import br.com.encomendo.core.domain.session.Role
import br.com.encomendo.core.domain.session.Session
import br.com.encomendo.core.domain.session.TenantSummary
import br.com.encomendo.core.domain.session.User

/** The store most test sessions select. */
val AltaRpm = TenantSummary(id = "0190f3c4-0000-7000-8000-000000000001", name = "Alta RPM", slug = "alta-rpm")

/**
 * Returns a signed-in session for tests: [userId] in [tenant] (null means no
 * store selected), granted [permissions] keys such as `order:write`.
 */
fun testSession(
    userId: String = "0190f3c4-0000-7000-8000-0000000000aa",
    tenant: TenantSummary? = AltaRpm,
    permissions: Set<String> = emptySet(),
): Session =
    Session(
        user = User(id = userId, name = "Ana Souza", email = "ana@altarpm.com.br"),
        tenant = tenant,
        role = tenant?.let { Role("owner") },
        permissions = GrantedPermissions(permissions.map(::Permission)),
        memberships = listOfNotNull(tenant?.let { Membership(it, Role("owner")) }),
    )
