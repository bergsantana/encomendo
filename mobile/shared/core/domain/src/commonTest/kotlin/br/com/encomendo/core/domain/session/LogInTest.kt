package br.com.encomendo.core.domain.session

import br.com.encomendo.core.domain.failure.Failure
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertSame

class LogInTest {
    @Test
    fun trimsTheEmailAndLeavesThePasswordAsTyped() =
        runTest {
            // Keyboards add a trailing space after an autocompleted email; a password's spaces are part of it.
            val auth = RecordingAuthRepository()

            LogIn(auth)(email = "  ana@altarpm.com.br ", password = " segredo forte ")

            assertEquals(listOf("ana@altarpm.com.br" to " segredo forte "), auth.attempts)
        }

    @Test
    fun returnsTheNewSession() =
        runTest {
            val auth = RecordingAuthRepository()

            assertSame(auth.session, LogIn(auth)(email = "ana@altarpm.com.br", password = "segredo-forte"))
        }

    @Test
    fun passesFailuresOn() =
        runTest {
            val wrong = Failure.WrongCredentials()
            val auth = RecordingAuthRepository(failure = wrong)

            val thrown = assertFailsWith<Failure> { LogIn(auth)(email = "ana@altarpm.com.br", password = "errada") }

            assertSame(wrong, thrown)
        }

    private class RecordingAuthRepository(
        private val failure: Failure? = null,
    ) : AuthRepository {
        val attempts = mutableListOf<Pair<String, String>>()
        val session =
            Session(
                user =
                    User(
                        id = "0190f3c4-0000-7000-8000-0000000000aa",
                        name = "Ana Souza",
                        email = "ana@altarpm.com.br",
                    ),
                tenant = null,
                role = null,
                permissions = GrantedPermissions.None,
                memberships = emptyList(),
            )

        override suspend fun logIn(
            email: String,
            password: String,
        ): Session {
            attempts += email to password
            failure?.let { throw it }
            return session
        }

        override suspend fun logOut() = error("LogIn never logs out")
    }
}
