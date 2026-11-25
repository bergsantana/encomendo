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
 *   if the server refuses the refresh token, the request fails with its 401
 *   and [tokens] is cleared, which signs the user out (if the store cannot be
 *   written, the next 401 tries again); if the refresh fails for any other
 *   reason, that error is thrown and the session is kept.
 *
 * Use one client for the whole app, so refreshes are never run twice in
 * parallel. Its refreshes run in [turns]; any other call that spends the
 * refresh token (switch-tenant, a refresh on app start) must take its turn
 * there too: two calls presenting one token leave one of them with a
 * superseded token, whose next use revokes the whole family (ADR 0013).
 */
fun <S : BearerCredentials> createApiClient(
    engine: HttpClientEngine,
    config: ApiClientConfig,
    tokens: TokenStorage<S>,
    refresher: TokenRefresher<S>,
    turns: SessionTurns,
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
        installBearer(
            apiOrigin = Url(config.baseUrl),
            refresh = BearerRefresh(tokens, refresher, turns),
            tokens = tokens,
            turns = turns,
        )
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

private fun <S : BearerCredentials> HttpClientConfig<*>.installBearer(
    apiOrigin: Url,
    refresh: BearerRefresh<S>,
    tokens: TokenStorage<S>,
    turns: SessionTurns,
) {
    install(Auth) {
        // A 401 from another host must never be answered with our token, and a 401 inside a
        // SessionTurns turn (the refresh call itself, a switch) must fail rather than wait for
        // another turn to refresh: turns are not reentrant.
        reAuthorizeOnResponse {
            it.status == HttpStatusCode.Unauthorized &&
                it.request.url.isSameOrigin(apiOrigin) &&
                !SessionTurns.inTurn()
        }
        bearer {
            // TokenStorage is the only copy, so a logout or tenant switch applies to the next request.
            cacheTokens = false
            // A cancelled screen must not drop a refresh the server already rotated: the old token is spent.
            nonCancellableRefresh = true
            loadTokens { tokenForTheScreens(tokens, turns)?.also(refresh::issued)?.toBearer() }
            sendWithoutRequest { it.url.build().isSameOrigin(apiOrigin) }
            refreshTokens { refresh.refresh(client, response.request.bearerToken())?.toBearer() }
        }
    }
}

/**
 * Returns the stored session once the app's screens serve its scope (ADR 0018).
 * A stored scope the screens don't serve means a store switch is under way:
 * the screens that started this request may be the old store's, about to be
 * cleared, so wait until the screens change, then read again. The screens'
 * scope is sampled before the read, so a change that lands in between is never
 * missed and the wait never outlives it.
 */
private suspend fun <S : BearerCredentials> tokenForTheScreens(
    tokens: TokenStorage<S>,
    turns: SessionTurns,
): S? {
    while (true) {
        val screens = turns.screensScope()
        val stored = tokens.read() ?: return null
        if (screens == null || stored.scope == screens) return stored
        turns.awaitScreensOtherThan(screens)
    }
}

private fun Url.isSameOrigin(other: Url): Boolean =
    protocol == other.protocol && host == other.host && port == other.port

private fun HttpRequest.bearerToken(): String? = headers[HttpHeaders.Authorization]?.removePrefix("Bearer ")

private fun BearerCredentials.toBearer(): BearerTokens =
    BearerTokens(accessToken = accessToken, refreshToken = refreshToken)
