package br.com.encomendo.core.network.tenancy

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
import io.ktor.client.engine.mock.toByteArray
import io.ktor.client.request.HttpRequestData
import io.ktor.http.HttpHeaders
import io.ktor.http.HttpMethod
import io.ktor.http.HttpStatusCode
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith

class TenancyApiTest {
    @Test
    fun createTenantSendsTheNameAndLinkWithTheSessionsToken() =
        runTest {
            // Works without a store selected (docs/api/openapi.yaml, createTenant): right after sign-up.
            val requests = mutableListOf<HttpRequestData>()
            val api =
                tenancyApi { request ->
                    requests += request
                    respondJson(TENANT_JSON, HttpStatusCode.Created)
                }

            val tenant = api.createTenant(name = "Alta RPM", slug = "alta-rpm")

            val request = requests.single()
            assertEquals(HttpMethod.Post, request.method)
            assertEquals("/api/v1/admin/tenants", request.url.encodedPath)
            assertEquals("Bearer access-no-store", request.headers[HttpHeaders.Authorization])
            assertEquals("""{"name":"Alta RPM","slug":"alta-rpm"}""", request.body.toByteArray().decodeToString())
            assertEquals("0190f3c4-0000-7000-8000-000000000001", tenant.id)
            assertEquals("Alta RPM", tenant.name)
            assertEquals("alta-rpm", tenant.slug)
        }

    @Test
    fun aTakenLinkIsAnApiError() =
        runTest {
            val api =
                tenancyApi {
                    respondProblem(
                        HttpStatusCode.Conflict,
                        """{"title":"Link em uso","status":409,"code":"slug_taken"}""",
                    )
                }

            val error = assertFailsWith<ApiError> { api.createTenant(name = "Alta RPM", slug = "alta-rpm") }

            assertEquals(ApiErrorCode.SlugTaken, error.code)
        }

    private fun tenancyApi(handler: MockRequestHandler): TenancyApi =
        TenancyApi(
            createApiClient(
                engine = MockEngine(handler),
                config = ApiClientConfig(baseUrl = API_URL),
                tokens =
                    InMemoryTokenStorage(
                        TestTokens(accessToken = "access-no-store", refreshToken = "refresh-old"),
                    ),
                refresher = TokenRefresher<TestTokens> { _, _ -> error("no refresh expected") },
                turns = SessionTurns(),
            ),
        )

    private companion object {
        /** A Tenant as docs/api/openapi.yaml describes it. */
        const val TENANT_JSON = """
            {"id": "0190f3c4-0000-7000-8000-000000000001", "name": "Alta RPM", "slug": "alta-rpm",
             "createdAt": "2026-11-25T12:00:00Z"}
        """
    }
}
