package br.com.encomendo.core.domain.session

import kotlin.test.Test
import kotlin.test.assertFalse
import kotlin.test.assertTrue

class SessionTest {
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
