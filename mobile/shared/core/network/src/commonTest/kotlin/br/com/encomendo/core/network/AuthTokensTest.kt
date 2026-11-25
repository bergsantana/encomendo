package br.com.encomendo.core.network

import kotlin.test.Test
import kotlin.test.assertFalse

class AuthTokensTest {
    @Test
    fun neverPrintsTheTokens() {
        // Product rule: tokens never reach a log or a crash report (mobile/CLAUDE.md, Auth and offline).
        val printed = AuthTokens(accessToken = "access-secret", refreshToken = "refresh-secret").toString()

        assertFalse(printed.contains("secret"), printed)
    }
}
