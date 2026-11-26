package br.com.encomendo.core.data.members

import br.com.encomendo.core.data.session.createSessionApiClient
import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.domain.members.Member
import br.com.encomendo.core.domain.session.Role
import br.com.encomendo.core.domain.session.scope
import br.com.encomendo.core.network.ApiClientConfig
import br.com.encomendo.core.network.SessionTurns
import br.com.encomendo.core.network.members.MembersApi
import br.com.encomendo.core.security.SessionTokens
import br.com.encomendo.core.security.StoredSession
import br.com.encomendo.core.security.TokenStore
import br.com.encomendo.core.testing.FakeSecretCipher
import br.com.encomendo.core.testing.temporaryPreferencesDataStore
import br.com.encomendo.core.testing.testSession
import io.ktor.client.engine.mock.MockEngine
import io.ktor.client.engine.mock.MockRequestHandleScope
import io.ktor.client.engine.mock.MockRequestHandler
import io.ktor.client.engine.mock.respond
import io.ktor.http.HttpHeaders
import io.ktor.http.HttpStatusCode
import io.ktor.http.headersOf
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNull

class ApiMembersRepositoryTest {
    @Test
    fun readsTheTeamAsMembers() =
        runTest {
            val members =
                repository {
                    respond(
                        """{"items": [$BRUNO_JSON]}""",
                        HttpStatusCode.OK,
                        headersOf(HttpHeaders.ContentType, "application/json"),
                    )
                }

            val page = members.page(cursor = null, limit = 100)

            assertEquals(listOf(BRUNO), page.members)
            assertNull(page.nextCursor)
        }

    @Test
    fun aChangedRoleComesBackAsSaved() =
        runTest {
            val members =
                repository {
                    respond(
                        BRUNO_JSON.replace("\"staff\"", "\"admin\""),
                        HttpStatusCode.OK,
                        headersOf(HttpHeaders.ContentType, "application/json"),
                    )
                }

            assertEquals(BRUNO.copy(role = Role.Admin), members.changeRole(userId = BRUNO.userId, role = Role.Admin))
        }

    @Test
    fun someoneNoLongerOnTheTeamIsNotFound() =
        runTest {
            // Removed by someone else meanwhile: Equipe says so at the top and reads the team again.
            val members = repository { problem(404, "not_found") }

            assertFailsWith<Failure.NotFound> { members.remove(userId = BRUNO.userId) }
        }

    @Test
    fun aLostPermissionIsForbidden() =
        runTest {
            val members = repository { problem(403, "forbidden") }

            assertFailsWith<Failure.Forbidden> { members.changeRole(userId = BRUNO.userId, role = Role.Viewer) }
        }

    private fun MockRequestHandleScope.problem(
        status: Int,
        code: String,
    ) = respond(
        """{"title":"Recusado","status":$status,"code":"$code"}""",
        HttpStatusCode.fromValue(status),
        headersOf(HttpHeaders.ContentType, "application/problem+json"),
    )

    private suspend fun TestScope.repository(handler: MockRequestHandler): ApiMembersRepository {
        val store =
            TokenStore(
                temporaryPreferencesDataStore(backgroundScope),
                FakeSecretCipher(),
                StandardTestDispatcher(testScheduler),
            )
        store.write(StoredSession(SessionTokens("access", "refresh"), testSession()))
        val turns = SessionTurns()
        // Only screens of the stored session's own scope may send its token (ADR 0018).
        turns.screensServe(testSession().scope)
        val client =
            createSessionApiClient(MockEngine(handler), ApiClientConfig(baseUrl = "https://api.test"), store, turns)
        return ApiMembersRepository(MembersApi(client))
    }

    private companion object {
        val BRUNO =
            Member(
                userId = "0190f3c4-0000-7000-8000-0000000000bb",
                name = "Bruno Lima",
                email = "bruno@altarpm.com.br",
                role = Role.Staff,
            )

        const val BRUNO_JSON = """
            {"userId": "0190f3c4-0000-7000-8000-0000000000bb", "name": "Bruno Lima", "email": "bruno@altarpm.com.br",
             "role": "staff", "joinedAt": "2026-11-02T12:00:00Z"}
        """
    }
}
