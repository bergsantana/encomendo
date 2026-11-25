package br.com.encomendo.core.domain.session

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

class SessionTest {
    @Test
    fun aScopeIsTheUserAndTheStore() {
        val user = User(id = "u-1", name = "Ana Souza", email = "ana@altarpm.com.br")
        val store = TenantSummary(id = "t-1", name = "Alta RPM", slug = "alta-rpm")
        val inStore = Session(user, store, Role("owner"), GrantedPermissions.None, emptyList())

        assertEquals("u-1@t-1", inStore.scope)
        assertEquals("u-1@", inStore.copy(tenant = null, role = null).scope, "no store is a scope of its own")
    }

    @Test
    fun neverPrintsTheUsersNameOrEmail() {
        // A session in a log line, crash report or failed test must not carry personal data.
        val user = User(id = "u-1", name = "Ana Souza", email = "ana@altarpm.com.br")
        val session =
            Session(
                user = user,
                tenant = null,
                role = null,
                permissions = GrantedPermissions.None,
                memberships = emptyList(),
            )

        for (printed in listOf(user.toString(), session.toString())) {
            assertFalse(printed.contains("Ana"), printed)
            assertFalse(printed.contains("altarpm"), printed)
            assertTrue(printed.contains("u-1"), printed)
        }
    }
}
