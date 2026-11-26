package br.com.encomendo.core.domain.session

import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

class PasswordResetTest {
    @Test
    fun aCodeIsAskedForTheTrimmedEmail() =
        runTest {
            val resets = RecordingResets()

            RequestPasswordReset(resets)(email = "  ana@altarpm.com.br ")

            assertEquals(listOf("ana@altarpm.com.br"), resets.requested)
        }

    @Test
    fun aCodeCopiedWithSpacesIsSentWithout() =
        runTest {
            // As the web does: a code copied from the email may come as "482 913".
            val resets = RecordingResets()

            ResetPassword(resets)(email = " ana@altarpm.com.br", code = " 482 913 ", password = " nova senha ")

            assertEquals(listOf(Triple("ana@altarpm.com.br", "482913", " nova senha ")), resets.resets)
        }

    @Test
    fun aCodeIsSixDigitsSpacesAside() {
        // The contract's rule (PasswordResetConfirmRequest.code), checked before sending, as the web does.
        listOf("482913", "482 913", " 482913 ").forEach { assertTrue(ResetPassword.isWellFormed(it), it) }
        listOf("", "48291", "4829130", "48291a", "482-913").forEach { assertFalse(ResetPassword.isWellFormed(it), it) }
    }

    private class RecordingResets : PasswordResetRepository {
        val requested = mutableListOf<String>()
        val resets = mutableListOf<Triple<String, String, String>>()

        override suspend fun requestCode(email: String) {
            requested += email
        }

        override suspend fun reset(
            email: String,
            code: String,
            password: String,
        ) {
            resets += Triple(email, code, password)
        }
    }
}
