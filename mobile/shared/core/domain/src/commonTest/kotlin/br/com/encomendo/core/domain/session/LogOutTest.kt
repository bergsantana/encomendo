package br.com.encomendo.core.domain.session

import br.com.encomendo.core.domain.failure.Failure
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertSame

class LogOutTest {
    @Test
    fun logsOutOnce() =
        runTest {
            val auth = CountingAuthRepository()

            LogOut(auth)()

            assertEquals(1, auth.logouts)
        }

    @Test
    fun passesFailuresOn() =
        runTest {
            val full = Failure.Unexpected(status = 0, code = "storage")
            val auth = CountingAuthRepository(failure = full)

            assertSame(full, assertFailsWith<Failure> { LogOut(auth)() })
        }

    private class CountingAuthRepository(
        private val failure: Failure? = null,
    ) : AuthRepository {
        var logouts = 0

        override suspend fun logIn(
            email: String,
            password: String,
        ): Session = error("LogOut never logs in")

        override suspend fun signUp(
            name: String,
            email: String,
            password: String,
        ): Session = error("LogOut never signs up")

        override suspend fun logOut() {
            logouts++
            failure?.let { throw it }
        }
    }
}
