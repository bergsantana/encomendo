package br.com.encomendo.core.network.auth

import br.com.encomendo.core.network.API_URL
import br.com.encomendo.core.network.ApiClientConfig
import br.com.encomendo.core.network.ApiError
import br.com.encomendo.core.network.ApiErrorCode
import br.com.encomendo.core.network.InMemoryTokenStorage
import br.com.encomendo.core.network.TestTokens
import br.com.encomendo.core.network.TokenRefresher
import br.com.encomendo.core.network.createApiClient
import br.com.encomendo.core.network.respondJson
import br.com.encomendo.core.network.respondProblem
import io.ktor.client.engine.mock.MockEngine
import io.ktor.client.engine.mock.MockRequestHandler
import io.ktor.client.engine.mock.toByteArray
import io.ktor.client.request.HttpRequestData
import io.ktor.http.ContentType
import io.ktor.http.HttpHeaders
import io.ktor.http.HttpMethod
import io.ktor.http.HttpStatusCode
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertFalse
import kotlin.test.assertNull

class AuthApiTest {
    @Test
    fun refreshSendsTheTokenInTheBodyAndNoBearer() =
        runTest {
            // Android uses the body refresh transport; the access token it replaces is not sent (ADR 0010).
            val requests = mutableListOf<HttpRequestData>()
            val api =
                authApi { request ->
                    requests += request
                    respondJson(SESSION_JSON)
                }

            api.refresh("refresh-old")

            val request = requests.single()
            assertEquals(HttpMethod.Post, request.method)
            assertEquals("/auth/refresh", request.url.encodedPath)
            assertNull(request.headers[HttpHeaders.Authorization])
            assertEquals(ContentType.Application.Json, request.body.contentType?.withoutParameters())
            assertEquals("""{"refreshToken":"refresh-old"}""", request.body.toByteArray().decodeToString())
        }

    @Test
    fun refreshReturnsTheWholeSession() =
        runTest {
            val api = authApi { respondJson(SESSION_JSON) }

            val session = api.refresh("refresh-old")

            assertEquals("access-new", session.accessToken)
            assertEquals("refresh-new", session.refreshToken)
            assertEquals("Ana Souza", session.user.name)
            assertEquals("alta-rpm", session.tenant?.slug)
            assertEquals("owner", session.role)
            assertEquals(listOf("order:read", "order:write"), session.permissions)
            assertEquals(
                "Moto Peças",
                session.memberships
                    .last()
                    .tenant.name,
            )
        }

    @Test
    fun refreshReadsASessionWithoutAStore() =
        runTest {
            val api = authApi { respondJson(NO_STORE_JSON) }

            val session = api.refresh("refresh-old")

            assertNull(session.tenant)
            assertNull(session.role)
            assertEquals(emptyList(), session.permissions)
        }

    @Test
    fun aRefusedRefreshIsAnApiError() =
        runTest {
            val api =
                authApi {
                    respondProblem(
                        HttpStatusCode.Unauthorized,
                        """{"type":"about:blank","title":"Sessão expirada","status":401,"code":"unauthenticated"}""",
                    )
                }

            val error = assertFailsWith<ApiError> { api.refresh("refresh-spent") }

            assertEquals(401, error.status)
            assertEquals(ApiErrorCode("unauthenticated"), error.code)
        }

    @Test
    fun neverPrintsTheTokens() =
        runTest {
            val session = authApi { respondJson(SESSION_JSON) }.refresh("refresh-old")

            assertFalse(session.toString().contains("access-new"), session.toString())
            assertFalse(session.toString().contains("refresh-new"), session.toString())
        }

    @Test
    fun aSessionWithoutARefreshTokenIsAnApiError() =
        runTest {
            // Android asks for the body transport, so this answer breaks the contract. It must
            // still fail as an ApiError, the one failure type callers handle.
            val api = authApi { respondJson(SESSION_JSON.replace(""""refreshToken": "refresh-new",""", "")) }

            val error = assertFailsWith<ApiError> { api.refresh("refresh-old") }

            assertEquals(200, error.status)
            assertEquals(ApiErrorCode.Unknown, error.code)
        }

    @Test
    fun anAnswerThatIsNotASessionIsAnApiError() =
        runTest {
            val api = authApi { respondJson("""{"status":"ok"}""") }

            val error = assertFailsWith<ApiError> { api.refresh("refresh-old") }

            assertEquals(ApiErrorCode.Unknown, error.code)
        }

    private fun authApi(handler: MockRequestHandler): AuthApi =
        AuthApi(
            createApiClient(
                engine = MockEngine(handler),
                config = ApiClientConfig(baseUrl = API_URL),
                // A stored session, so a missing withoutBearer() would show up as a header.
                tokens = InMemoryTokenStorage(TestTokens(accessToken = "access-old", refreshToken = "refresh-old")),
                refresher = TokenRefresher<TestTokens> { _, _ -> error("the refresh call must never refresh") },
            ),
        )

    private companion object {
        /** An AuthSession as docs/api/openapi.yaml describes it, plus a field the app does not know yet. */
        const val SESSION_JSON = """
            {
              "accessToken": "access-new",
              "tokenType": "Bearer",
              "expiresIn": 900,
              "refreshToken": "refresh-new",
              "user": {"id": "0190f3c4-0000-7000-8000-0000000000aa", "name": "Ana Souza", "email": "ana@altarpm.com.br"},
              "tenant": {"id": "0190f3c4-0000-7000-8000-000000000001", "name": "Alta RPM", "slug": "alta-rpm"},
              "role": "owner",
              "permissions": ["order:read", "order:write"],
              "memberships": [
                {"tenant": {"id": "0190f3c4-0000-7000-8000-000000000001", "name": "Alta RPM", "slug": "alta-rpm"}, "role": "owner"},
                {"tenant": {"id": "0190f3c4-0000-7000-8000-000000000002", "name": "Moto Peças", "slug": "moto-pecas"}, "role": "staff"}
              ],
              "plan": "added later"
            }
        """

        /** Right after sign-up: no store selected, so no role and no permissions. */
        const val NO_STORE_JSON = """
            {
              "accessToken": "access-new",
              "tokenType": "Bearer",
              "expiresIn": 900,
              "refreshToken": "refresh-new",
              "user": {"id": "0190f3c4-0000-7000-8000-0000000000aa", "name": "Ana Souza", "email": "ana@altarpm.com.br"},
              "permissions": [],
              "memberships": []
            }
        """
    }
}
