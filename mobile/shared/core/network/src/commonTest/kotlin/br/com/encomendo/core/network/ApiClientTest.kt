package br.com.encomendo.core.network

import io.ktor.client.HttpClient
import io.ktor.client.call.body
import io.ktor.client.engine.mock.MockEngine
import io.ktor.client.engine.mock.MockRequestHandler
import io.ktor.client.engine.mock.respond
import io.ktor.client.engine.mock.toByteArray
import io.ktor.client.request.get
import io.ktor.client.request.post
import io.ktor.client.request.setBody
import io.ktor.http.ContentType
import io.ktor.http.HttpHeaders
import io.ktor.http.HttpStatusCode
import io.ktor.http.contentType
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitCancellation
import kotlinx.coroutines.test.runTest
import kotlinx.io.IOException
import kotlinx.serialization.Serializable
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNull
import kotlin.test.assertTrue

class ApiClientTest {
    @Test
    fun decodesJsonBodies() =
        runTest {
            val client = client { respondJson("""{"status":"ok","extra":"ignored"}""") }

            assertEquals(Health(status = "ok"), client.get("/readyz").body<Health>())
        }

    @Test
    fun sendsJsonBodiesAndAcceptsProblemJson() =
        runTest {
            var seenBody = ""
            var seenAccept = emptyList<String>()
            val client =
                client { request ->
                    seenBody = request.body.toByteArray().decodeToString()
                    seenAccept = request.headers.getAll(HttpHeaders.Accept).orEmpty()
                    respond("", HttpStatusCode.NoContent)
                }

            client.post("/echo") {
                contentType(ContentType.Application.Json)
                setBody(Health(status = "ok"))
            }

            assertEquals("""{"status":"ok"}""", seenBody)
            assertTrue(seenAccept.joinToString().contains("application/problem+json"), "Accept was $seenAccept")
        }

    @Test
    fun turnsProblemJsonIntoApiErrorWithCodeAndFieldIssues() =
        runTest {
            val client =
                client {
                    respondProblem(
                        status = HttpStatusCode.BadRequest,
                        body =
                            """
                            {"type":"about:blank","title":"Dados inválidos","status":400,"code":"invalid",
                             "detail":"Confira os campos.",
                             "errors":[{"field":"phone","message":"Informe um WhatsApp válido."}]}
                            """.trimIndent(),
                    )
                }

            val error = assertFailsWith<ApiError> { client.post("/orders") }

            assertEquals(400, error.status)
            assertEquals(ApiErrorCode.Invalid, error.code)
            assertEquals("Dados inválidos", error.title)
            assertEquals("Confira os campos.", error.detail)
            assertEquals(
                listOf(FieldIssue(field = "phone", message = "Informe um WhatsApp válido.")),
                error.fieldIssues,
            )
        }

    @Test
    fun keepsCodesThisAppVersionDoesNotKnow() =
        runTest {
            val client =
                client {
                    respondProblem(
                        HttpStatusCode.Conflict,
                        """{"title":"Limite do plano","status":409,"code":"plan_limit_reached"}""",
                    )
                }

            val error = assertFailsWith<ApiError> { client.get("/orders/1") }

            assertEquals(ApiErrorCode("plan_limit_reached"), error.code)
        }

    @Test
    fun readsTheWaitARateLimitAsksFor() =
        runTest {
            // Android shows the wait from Retry-After, which every 429 carries (ADR 0013).
            val client = client { respondRateLimited(retryAfter = "42") }

            val error = assertFailsWith<ApiError> { client.post("/auth/login") }

            assertEquals(429, error.status)
            assertEquals(ApiErrorCode.RateLimited, error.code)
            assertEquals(42, error.retryAfterSeconds)
        }

    @Test
    fun aWaitThatIsNotWholeSecondsIsUnknown() =
        runTest {
            // The contract promises whole seconds, at least 1. An HTTP date or a broken
            // value reads as "no wait given" rather than a wrong number on screen.
            for (header in listOf("Wed, 25 Nov 2026 10:00:00 GMT", "0", "-5", "1.5", "")) {
                val client = client { respondRateLimited(retryAfter = header) }

                val error = assertFailsWith<ApiError> { client.post("/auth/login") }

                assertNull(error.retryAfterSeconds, "Retry-After: '$header'")
            }
        }

    @Test
    fun anErrorWithoutAWaitHasNone() =
        runTest {
            val client = client { respond("Bad Gateway", HttpStatusCode.BadGateway) }

            assertNull(assertFailsWith<ApiError> { client.get("/broken") }.retryAfterSeconds)
        }

    @Test
    fun handlesErrorBodiesThatAreNotProblems() =
        runTest {
            val client = client { respond("Bad Gateway", HttpStatusCode.BadGateway) }

            val error = assertFailsWith<ApiError> { client.get("/broken") }

            assertEquals(502, error.status)
            assertEquals(ApiErrorCode.Unknown, error.code)
            assertNull(error.title)
            assertEquals(emptyList(), error.fieldIssues)
        }

    @Test
    fun reportsAnUnreachableServerAsANetworkError() =
        runTest {
            val client = client { throw IOException("connection refused") }

            val error = assertFailsWith<ApiError> { client.get("/readyz") }

            assertEquals(0, error.status)
            assertEquals(ApiErrorCode.Network, error.code)
        }

    @Test
    fun leavesCancellationAsCancellation() =
        runTest {
            val started = CompletableDeferred<Unit>()
            val client =
                client {
                    started.complete(Unit)
                    awaitCancellation()
                }

            val call = async { client.get("/slow") }
            started.await()
            call.cancel()

            assertFailsWith<CancellationException> { call.await() }
        }

    @Serializable
    private data class Health(
        val status: String,
    )

    private fun client(handler: MockRequestHandler): HttpClient =
        createApiClient(
            engine = MockEngine(handler),
            config = ApiClientConfig(baseUrl = API_URL),
            tokens = InMemoryTokenStorage(),
            refresher = TokenRefresher<TestTokens> { _, _ -> error("no refresh expected") },
            turns = SessionTurns(),
        )
}
