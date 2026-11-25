package br.com.encomendo.core.data.session

import br.com.encomendo.core.domain.session.GrantedPermissions
import br.com.encomendo.core.domain.session.Membership
import br.com.encomendo.core.domain.session.Permission
import br.com.encomendo.core.domain.session.Role
import br.com.encomendo.core.domain.session.TenantSummary
import br.com.encomendo.core.domain.session.User
import br.com.encomendo.core.network.auth.AuthSessionDto
import br.com.encomendo.core.network.auth.MembershipDto
import br.com.encomendo.core.network.auth.TenantSummaryDto
import br.com.encomendo.core.network.auth.UserDto
import br.com.encomendo.core.security.SessionTokens
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertNull

class SessionMappingTest {
    private val altaRpm = TenantSummaryDto(id = "t-1", name = "Alta RPM", slug = "alta-rpm")
    private val user = UserDto(id = "u-1", name = "Ana Souza", email = "ana@altarpm.com.br")

    @Test
    fun mapsEverySessionField() {
        val dto =
            AuthSessionDto(
                accessToken = "access",
                refreshToken = "refresh",
                user = user,
                tenant = altaRpm,
                role = "staff",
                permissions = listOf("order:read", "receiving:write"),
                memberships = listOf(MembershipDto(altaRpm, "staff")),
            )

        val stored = dto.toStoredSession()

        assertEquals(SessionTokens(accessToken = "access", refreshToken = "refresh"), stored.tokens)
        val session = stored.session
        assertEquals(User(id = "u-1", name = "Ana Souza", email = "ana@altarpm.com.br"), session.user)
        val tenant = assertNotNull(session.tenant)
        assertEquals(TenantSummary(id = "t-1", name = "Alta RPM", slug = "alta-rpm"), tenant)
        assertEquals(Role("staff"), session.role)
        assertEquals(
            GrantedPermissions(listOf(Permission("order:read"), Permission("receiving:write"))),
            session.permissions,
        )
        assertEquals(listOf(Membership(tenant, Role("staff"))), session.memberships)
    }

    @Test
    fun mapsASessionWithoutAStore() {
        val dto =
            AuthSessionDto(
                accessToken = "access",
                refreshToken = "refresh",
                user = user,
                permissions = emptyList(),
                memberships = emptyList(),
            )

        val session = dto.toStoredSession().session

        assertNull(session.tenant)
        assertNull(session.role)
        assertEquals(GrantedPermissions.None, session.permissions)
    }
}
