package br.com.encomendo.core.data.session

import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.network.ApiClientConfig
import br.com.encomendo.core.network.SessionTurns
import br.com.encomendo.core.network.auth.AuthApi
import br.com.encomendo.core.security.TokenStore
import br.com.encomendo.core.testing.FakeSecretCipher
import br.com.encomendo.core.testing.temporaryPreferencesDataStore
import io.ktor.client.engine.mock.MockEngine
import io.ktor.client.engine.mock.MockRequestHandler
import io.ktor.client.engine.mock.respond
import io.ktor.http.HttpHeaders
import io.ktor.http.HttpStatusCode
import io.ktor.http.headersOf
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runTest
import kotlinx.io.IOException
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith

class ApiPasswordResetRepositoryTest {
    @Test
    fun asksForACodeAndSetsTheNewPassword() =
        runTest {
            val paths = mutableListOf<String>()
            val resets =
                repository { request ->
                    paths += request.url.encodedPath
                    respond("", if (paths.size == 1) HttpStatusCode.Accepted else HttpStatusCode.NoContent)
                }

            resets.requestCode(email = "ana@altarpm.com.br")
            resets.reset(email = "ana@altarpm.com.br", code = "482913", password = "nova-senha-forte")

            assertEquals(listOf("/auth/password-reset", "/auth/password-reset/confirm"), paths)
        }

    @Test
    fun aWrongCodeIsAWrongCode() =
        runTest {
            val resets =
                repository {
                    respond(
                        """{"title":"Código inválido","status":400,"code":"invalid_code"}""",
                        HttpStatusCode.BadRequest,
                        headersOf(HttpHeaders.ContentType, "application/problem+json"),
                    )
                }

            assertFailsWith<Failure.WrongCode> { resets.reset("ana@altarpm.com.br", "000000", "nova-senha-forte") }
        }

    @Test
    fun noSignalIsOffline() =
        runTest {
            val resets = repository { throw IOException("no signal") }

            assertFailsWith<Failure.Offline> { resets.requestCode(email = "ana@altarpm.com.br") }
        }

    private fun TestScope.repository(handler: MockRequestHandler): ApiPasswordResetRepository {
        // Signed out: these calls carry no token, and a reset never signs in (the user logs in after).
        val store =
            TokenStore(
                temporaryPreferencesDataStore(backgroundScope),
                FakeSecretCipher(),
                StandardTestDispatcher(testScheduler),
            )
        val client =
            createSessionApiClient(
                MockEngine(handler),
                ApiClientConfig(baseUrl = "https://api.test"),
                store,
                SessionTurns(),
            )
        return ApiPasswordResetRepository(AuthApi(client))
    }
}
