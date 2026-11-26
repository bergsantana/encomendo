package br.com.encomendo.core.network.auth

import br.com.encomendo.core.network.decoded
import br.com.encomendo.core.network.withoutBearer
import io.ktor.client.HttpClient
import io.ktor.client.request.bearerAuth
import io.ktor.client.request.post
import io.ktor.client.request.setBody
import io.ktor.client.statement.HttpResponse
import io.ktor.http.ContentType
import io.ktor.http.contentType
import kotlinx.serialization.Serializable

/**
 * AuthApi calls the `/auth` endpoints (docs/api/openapi.yaml, ADR 0010).
 * Android always uses the `body` refresh transport: refresh tokens travel in
 * JSON bodies, never in cookies. No call carries the stored access token or
 * triggers a refresh; logOut sends only the tokens it is given. An answer that
 * isn't 2xx throws ApiError.
 */
class AuthApi(
    private val client: HttpClient,
) {
    /**
     * Logs in with [email] and [password] and returns the new session, with
     * its refresh token in the body. [tenantId], the store used last on this
     * device, is selected when the user still belongs to it. Wrong credentials throw ApiError with
     * code `invalid_credentials`: a form error, never a reason to refresh.
     */
    suspend fun logIn(
        email: String,
        password: String,
        tenantId: String? = null,
    ): AuthSessionDto =
        client
            .post("/auth/login") {
                withoutBearer()
                contentType(ContentType.Application.Json)
                setBody(LoginRequest(email, password, tenantId, refreshTransport = BODY_TRANSPORT))
            }.session()

    /**
     * Creates a staff account and returns its session, with no store yet and
     * its refresh token in the body. An email already in use throws ApiError
     * with code `email_taken`.
     */
    suspend fun signUp(
        name: String,
        email: String,
        password: String,
    ): AuthSessionDto =
        client
            .post("/auth/signup") {
                withoutBearer()
                contentType(ContentType.Application.Json)
                setBody(SignUpRequest(name, email, password, refreshTransport = BODY_TRANSPORT))
            }.session()

    /**
     * Spends [refreshToken] for a session in the store [tenantId] (ADR 0010).
     * Like [refresh], call it only in a `SessionTurns` turn. A 400 or 403
     * leaves the refresh token as it was (ADR 0013); a 401 means it was refused.
     */
    suspend fun switchTenant(
        refreshToken: String,
        tenantId: String,
    ): AuthSessionDto =
        client
            .post("/auth/switch-tenant") {
                withoutBearer()
                contentType(ContentType.Application.Json)
                setBody(SwitchTenantRequest(tenantId = tenantId, refreshToken = refreshToken))
            }.session()

    /**
     * Asks the server to end the login: [refreshToken] names its family, which
     * is revoked, and [accessToken] stops working at once (ADR 0010). Both are
     * passed in because the device has already forgotten them (ADR 0016), and
     * the stored ones, if any, belong to whoever logged in since.
     */
    suspend fun logOut(
        accessToken: String,
        refreshToken: String,
    ) {
        client.post("/auth/logout") {
            withoutBearer()
            bearerAuth(accessToken)
            contentType(ContentType.Application.Json)
            setBody(LogoutRequest(refreshToken))
        }
    }

    /**
     * Trades [refreshToken] for a new session. The token is spent once the
     * server reads it, even if the answer never arrives, so call this only in
     * a `SessionTurns` turn, as the client's own refresh does.
     */
    suspend fun refresh(refreshToken: String): AuthSessionDto =
        client
            .post("/auth/refresh") {
                withoutBearer()
                contentType(ContentType.Application.Json)
                setBody(RefreshRequest(refreshToken))
            }.session()
}

/** Reads an AuthSession; see [decoded] for an answer that is not one. */
private suspend fun HttpResponse.session(): AuthSessionDto = decoded()

/** Asks for the refresh token in the response body instead of a cookie (docs/api/openapi.yaml, `RefreshTransport`). */
private const val BODY_TRANSPORT = "body"

/** Not a data class, so the password never reaches a log through toString. */
@Serializable
private class LoginRequest(
    val email: String,
    val password: String,
    /** The store used last on this device; left out when null. */
    val tenantId: String?,
    // No default: ApiJson leaves defaults out, and the server would fall back to the cookie.
    val refreshTransport: String,
)

/** Not a data class, so the password never reaches a log through toString. */
@Serializable
private class SignUpRequest(
    val name: String,
    val email: String,
    val password: String,
    // No default: ApiJson leaves defaults out, and the server would fall back to the cookie.
    val refreshTransport: String,
)

/** Not a data class, so the token never reaches a log through toString. */
@Serializable
private class SwitchTenantRequest(
    val tenantId: String,
    val refreshToken: String,
)

/** Not a data class, so the token never reaches a log through toString. */
@Serializable
private class LogoutRequest(
    val refreshToken: String,
)

@Serializable
private class RefreshRequest(
    val refreshToken: String,
)
