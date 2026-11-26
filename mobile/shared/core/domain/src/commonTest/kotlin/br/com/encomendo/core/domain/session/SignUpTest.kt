package br.com.encomendo.core.domain.session

import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals

class SignUpTest {
    @Test
    fun trimsTheNameAndEmailAndLeavesThePasswordAsTyped() =
        runTest {
            // Keyboards add a trailing space after autocomplete; a password's spaces are part of it.
            val auth = RecordingAuthRepository()

            SignUp(auth)(name = " Ana Souza ", email = "  ana@altarpm.com.br ", password = " segredo forte ")

            assertEquals(listOf(Triple("Ana Souza", "ana@altarpm.com.br", " segredo forte ")), auth.signUps)
        }

    private class RecordingAuthRepository : AuthRepository {
        val signUps = mutableListOf<Triple<String, String, String>>()

        override suspend fun logIn(
            email: String,
            password: String,
        ): Session = error("SignUp never logs in")

        override suspend fun signUp(
            name: String,
            email: String,
            password: String,
        ): Session {
            signUps += Triple(name, email, password)
            return Session(
                user = User(id = "u-1", name = name, email = email),
                tenant = null,
                role = null,
                permissions = GrantedPermissions.None,
                memberships = emptyList(),
            )
        }

        override suspend fun logOut() = error("SignUp never logs out")
    }
}
