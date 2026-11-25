package br.com.encomendo.core.network

import io.ktor.client.HttpClient
import io.ktor.client.engine.mock.MockRequestHandleScope
import io.ktor.client.engine.mock.respond
import io.ktor.client.request.HttpResponseData
import io.ktor.http.ContentType
import io.ktor.http.HttpHeaders
import io.ktor.http.HttpStatusCode
import io.ktor.http.headersOf
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.update

internal const val API_URL = "https://api.test"

internal fun MockRequestHandleScope.respondJson(
    body: String,
    status: HttpStatusCode = HttpStatusCode.OK,
): HttpResponseData = respond(body, status, headersOf(HttpHeaders.ContentType, ContentType.Application.Json.toString()))

internal fun MockRequestHandleScope.respondProblem(
    status: HttpStatusCode,
    body: String,
): HttpResponseData = respond(body, status, headersOf(HttpHeaders.ContentType, "application/problem+json"))

internal fun MockRequestHandleScope.respondUnauthorized(): HttpResponseData =
    respondProblem(HttpStatusCode.Unauthorized, """{"title":"Sessão expirada","status":401,"code":"unauthenticated"}""")

/** A token pair and nothing else: what the client needs from a stored session. */
internal data class TestTokens(
    override val accessToken: String,
    override val refreshToken: String,
) : BearerCredentials

/** Keeps tokens in memory, as TokenStore does on the device; thread-safe because MockEngine runs on a pool. */
internal class InMemoryTokenStorage(
    initial: TestTokens? = null,
) : TokenStorage<TestTokens> {
    private val state = MutableStateFlow(initial)

    /** The tokens stored right now. */
    val current: TestTokens? get() = state.value

    /** Replaces the tokens from outside the client, as a login or logout screen would. */
    fun overwrite(tokens: TestTokens?) {
        state.value = tokens
    }

    override suspend fun read(): TestTokens? = state.value

    override suspend fun replace(
        expected: TestTokens,
        replacement: TestTokens?,
    ): Boolean = state.compareAndSet(expected, replacement)
}

/** Records every refresh and answers with [answer]. */
internal class RecordingRefresher(
    private val answer: suspend (refreshToken: String) -> TestTokens,
) : TokenRefresher<TestTokens> {
    private val seen = MutableStateFlow(emptyList<String>())

    /** The refresh tokens sent so far, in order. */
    val calls: List<String> get() = seen.value

    override suspend fun refresh(
        client: HttpClient,
        current: TestTokens,
    ): TestTokens {
        seen.update { it + current.refreshToken }
        return answer(current.refreshToken)
    }
}
