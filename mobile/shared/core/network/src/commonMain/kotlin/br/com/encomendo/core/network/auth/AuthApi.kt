package br.com.encomendo.core.network.auth

import br.com.encomendo.core.network.ApiError
import br.com.encomendo.core.network.ApiErrorCode
import br.com.encomendo.core.network.withoutBearer
import io.ktor.client.HttpClient
import io.ktor.client.call.NoTransformationFoundException
import io.ktor.client.call.body
import io.ktor.client.request.post
import io.ktor.client.request.setBody
import io.ktor.client.statement.HttpResponse
import io.ktor.http.ContentType
import io.ktor.http.contentType
import io.ktor.serialization.ContentConvertException
import kotlinx.serialization.Serializable

/**
 * AuthApi calls the `/auth` endpoints (docs/api/openapi.yaml, ADR 0010).
 * Android always uses the `body` refresh transport: refresh tokens travel in
 * JSON bodies, never in cookies. Every call carries no access token and never
 * triggers a refresh; an answer that isn't 2xx throws ApiError.
 */
class AuthApi(
    private val client: HttpClient,
) {
    /**
     * Trades [refreshToken] for a new session. The token is spent once the
     * server reads it, even if the answer never arrives, so call this only
     * through the client's serialized refresh.
     */
    suspend fun refresh(refreshToken: String): AuthSessionDto =
        client
            .post("/auth/refresh") {
                withoutBearer()
                contentType(ContentType.Application.Json)
                setBody(RefreshRequest(refreshToken))
            }.session()
}

/**
 * Reads an AuthSession. An answer that does not match the contract is an
 * ApiError with code `unknown`, the one failure type callers handle, rather
 * than a decoding exception no screen expects.
 */
private suspend fun HttpResponse.session(): AuthSessionDto =
    try {
        body()
    } catch (error: ContentConvertException) {
        throw ApiError(status = status.value, code = ApiErrorCode.Unknown, cause = error)
    } catch (error: NoTransformationFoundException) {
        throw ApiError(status = status.value, code = ApiErrorCode.Unknown, cause = error)
    }

@Serializable
private class RefreshRequest(
    val refreshToken: String,
)
