package br.com.encomendo.core.network.members

import br.com.encomendo.core.network.API_URL
import br.com.encomendo.core.network.ApiClientConfig
import br.com.encomendo.core.network.ApiError
import br.com.encomendo.core.network.ApiErrorCode
import br.com.encomendo.core.network.InMemoryTokenStorage
import br.com.encomendo.core.network.SessionTurns
import br.com.encomendo.core.network.TestTokens
import br.com.encomendo.core.network.TokenRefresher
import br.com.encomendo.core.network.createApiClient
import br.com.encomendo.core.network.respondJson
import br.com.encomendo.core.network.respondProblem
import io.ktor.client.engine.mock.MockEngine
import io.ktor.client.engine.mock.MockRequestHandler
import io.ktor.client.engine.mock.respond
import io.ktor.client.engine.mock.toByteArray
import io.ktor.client.request.HttpRequestData
import io.ktor.http.HttpHeaders
import io.ktor.http.HttpMethod
import io.ktor.http.HttpStatusCode
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNull

class MembersApiTest {
    @Test
    fun listsOnePageOfTheTeamWithTheSessionsToken() =
        runTest {
            val requests = mutableListOf<HttpRequestData>()
            val api =
                membersApi { request ->
                    requests += request
                    respondJson("""{"items": [$ANA_JSON], "nextCursor": "c2"}""")
                }

            val page = api.list(cursor = null, limit = 100)

            val request = requests.single()
            assertEquals(HttpMethod.Get, request.method)
            assertEquals("/api/v1/admin/members", request.url.encodedPath)
            assertEquals("100", request.url.parameters["limit"])
            assertNull(request.url.parameters["cursor"], "the first page sends no cursor")
            assertEquals("Bearer access-alta-rpm", request.headers[HttpHeaders.Authorization])
            val ana = page.items.single()
            assertEquals(ANA_ID, ana.userId)
            assertEquals("Ana Souza", ana.name)
            assertEquals("ana@altarpm.com.br", ana.email)
            assertEquals("owner", ana.role)
            assertEquals("c2", page.nextCursor)
        }

    @Test
    fun theNextPageSendsTheCursorAndTheLastHasNone() =
        runTest {
            val requests = mutableListOf<HttpRequestData>()
            val api =
                membersApi { request ->
                    requests += request
                    respondJson("""{"items": []}""")
                }

            val page = api.list(cursor = "c2", limit = 100)

            assertEquals("c2", requests.single().url.parameters["cursor"])
            assertNull(page.nextCursor)
        }

    @Test
    fun changesARoleAndReturnsTheMember() =
        runTest {
            val requests = mutableListOf<HttpRequestData>()
            val api =
                membersApi { request ->
                    requests += request
                    respondJson(BRUNO_JSON.replace("\"staff\"", "\"viewer\""))
                }

            val changed = api.changeRole(userId = BRUNO_ID, role = "viewer")

            val request = requests.single()
            assertEquals(HttpMethod.Put, request.method)
            assertEquals("/api/v1/admin/members/$BRUNO_ID/role", request.url.encodedPath)
            assertEquals("""{"role":"viewer"}""", request.body.toByteArray().decodeToString())
            assertEquals("viewer", changed.role)
        }

    @Test
    fun removesSomeone() =
        runTest {
            val requests = mutableListOf<HttpRequestData>()
            val api =
                membersApi { request ->
                    requests += request
                    respond("", HttpStatusCode.NoContent)
                }

            api.remove(userId = BRUNO_ID)

            val request = requests.single()
            assertEquals(HttpMethod.Delete, request.method)
            assertEquals("/api/v1/admin/members/$BRUNO_ID", request.url.encodedPath)
        }

    @Test
    fun aUserIdNeverLeavesItsPathSegment() =
        runTest {
            // Ids come from the server, but a path built from one must still name only that member.
            val requests = mutableListOf<HttpRequestData>()
            val api =
                membersApi { request ->
                    requests += request
                    respond("", HttpStatusCode.NoContent)
                }

            api.remove(userId = "../tenants")

            assertEquals("/api/v1/admin/members/..%2Ftenants", requests.single().url.encodedPath)
        }

    @Test
    fun someoneNoLongerOnTheTeamIsAnApiError() =
        runTest {
            val api =
                membersApi {
                    respondProblem(
                        HttpStatusCode.NotFound,
                        """{"title":"Não encontrado","status":404,"code":"not_found"}""",
                    )
                }

            val error = assertFailsWith<ApiError> { api.changeRole(userId = BRUNO_ID, role = "admin") }

            assertEquals(ApiErrorCode.NotFound, error.code)
        }

    private fun membersApi(handler: MockRequestHandler): MembersApi =
        MembersApi(
            createApiClient(
                engine = MockEngine(handler),
                config = ApiClientConfig(baseUrl = API_URL),
                tokens =
                    InMemoryTokenStorage(
                        TestTokens(accessToken = "access-alta-rpm", refreshToken = "refresh-old"),
                    ),
                refresher = TokenRefresher<TestTokens> { _, _ -> error("no refresh expected") },
                turns = SessionTurns(),
            ),
        )

    private companion object {
        const val ANA_ID = "0190f3c4-0000-7000-8000-0000000000aa"
        const val BRUNO_ID = "0190f3c4-0000-7000-8000-0000000000bb"

        /** Members as docs/api/openapi.yaml describes them. */
        const val ANA_JSON = """
            {"userId": "$ANA_ID", "name": "Ana Souza", "email": "ana@altarpm.com.br", "role": "owner",
             "joinedAt": "2026-11-01T12:00:00Z"}
        """
        const val BRUNO_JSON = """
            {"userId": "$BRUNO_ID", "name": "Bruno Lima", "email": "bruno@altarpm.com.br", "role": "staff",
             "joinedAt": "2026-11-02T12:00:00Z"}
        """
    }
}
