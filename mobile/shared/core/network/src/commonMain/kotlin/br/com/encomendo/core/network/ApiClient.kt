package br.com.encomendo.core.network

import io.ktor.client.HttpClient
import io.ktor.client.HttpClientConfig
import io.ktor.client.engine.HttpClientEngine
import io.ktor.client.plugins.HttpResponseValidator
import io.ktor.client.plugins.HttpTimeout
import io.ktor.client.plugins.auth.Auth
import io.ktor.client.plugins.auth.AuthCircuitBreaker
import io.ktor.client.plugins.auth.providers.BearerTokens
import io.ktor.client.plugins.auth.providers.bearer
import io.ktor.client.plugins.contentnegotiation.ContentNegotiation
import io.ktor.client.plugins.defaultRequest
import io.ktor.client.request.HttpRequest
import io.ktor.client.request.HttpRequestBuilder
import io.ktor.client.request.accept
import io.ktor.client.statement.request
import io.ktor.http.ContentType
import io.ktor.http.HttpHeaders
import io.ktor.http.HttpStatusCode
import io.ktor.http.Url
import io.ktor.http.isSuccess
import io.ktor.serialization.kotlinx.json.json
import kotlinx.io.IOException
import kotlinx.serialization.json.Json

/**
 * The JSON settings for every API body: unknown fields are ignored, because
 * the server may add fields before the app knows them.
 */
internal val ApiJson: Json =
    Json {
        ignoreUnknownKeys = true
        explicitNulls = false
    }

/**
 * Creates the HTTP client for the Encomendo API. It guarantees that:
 *
 * - every non-2xx response throws [ApiError] read from its problem+json body,
 *   and a request that never reached the server throws [ApiError] with
 *   [ApiErrorCode.Network];
 * - the access token from [tokens] goes only to the [ApiClientConfig.baseUrl]
 *   origin (same scheme, host and port), so with an https base URL it never
 *   travels over plain HTTP or to another host;
 * - a 401 from the API triggers one refresh through [refresher] and one retry;
 *   if the server refuses the refresh token, [tokens] is cleared, which signs
 *   the user out; if the refresh fails for any other reason, that error is
 *   thrown and the session is kept.
 *
 * Use one client for the whole app, so refreshes are never run twice in parallel.
 */
fun createApiClient(
    engine: HttpClientEngine,
    config: ApiClientConfig,
    tokens: TokenStorage,
    refresher: TokenRefresher,
): HttpClient =
    HttpClient(engine) {
        install(ContentNegotiation) { json(ApiJson) }
        install(HttpTimeout) {
            connectTimeoutMillis = config.connectTimeoutMillis
            requestTimeoutMillis = config.requestTimeoutMillis
        }
        defaultRequest {
            url(config.baseUrl)
            accept(ContentType.Application.ProblemJson)
        }
        installErrorMapping()
        installBearer(apiOrigin = Url(config.baseUrl), refresh = BearerRefresh(tokens, refresher), tokens = tokens)
    }

/**
 * Marks a request that must carry no access token and must not trigger a
 * refresh on 401: login, sign-up and the refresh call itself.
 */
fun HttpRequestBuilder.withoutBearer() {
    attributes.put(AuthCircuitBreaker, Unit)
}

private fun HttpClientConfig<*>.installErrorMapping() {
    HttpResponseValidator {
        validateResponse { response ->
            if (!response.status.isSuccess()) throw response.toApiError(ApiJson)
        }
        handleResponseExceptionWithRequest { cause, _ ->
            // Timeouts are IOExceptions too. CancellationException is not, so it passes through.
            if (cause is IOException) throw ApiError(status = 0, code = ApiErrorCode.Network, cause = cause)
        }
    }
}

private fun HttpClientConfig<*>.installBearer(
    apiOrigin: Url,
    refresh: BearerRefresh,
    tokens: TokenStorage,
) {
    install(Auth) {
        // A 401 from another host must never be answered with our token, and a 401
        // on the refresh call itself must fail instead of refreshing inside the refresh.
        reAuthorizeOnResponse {
            it.status == HttpStatusCode.Unauthorized &&
                it.request.url.isSameOrigin(apiOrigin) &&
                !BearerRefresh.isRunning()
        }
        bearer {
            // TokenStorage is the only copy, so a logout or tenant switch applies to the next request.
            cacheTokens = false
            // A cancelled screen must not drop a refresh the server already rotated: the old token is spent.
            nonCancellableRefresh = true
            loadTokens { tokens.read()?.toBearer() }
            sendWithoutRequest { it.url.build().isSameOrigin(apiOrigin) }
            refreshTokens { refresh.refresh(client, response.request.bearerToken())?.toBearer() }
        }
    }
}

private fun Url.isSameOrigin(other: Url): Boolean =
    protocol == other.protocol && host == other.host && port == other.port

private fun HttpRequest.bearerToken(): String? = headers[HttpHeaders.Authorization]?.removePrefix("Bearer ")

private fun AuthTokens.toBearer(): BearerTokens = BearerTokens(accessToken = accessToken, refreshToken = refreshToken)
