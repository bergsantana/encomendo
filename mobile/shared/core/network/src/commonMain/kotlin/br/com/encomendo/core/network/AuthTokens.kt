package br.com.encomendo.core.network

import io.ktor.client.HttpClient

/**
 * AuthTokens is the signed-in user's token pair (docs/architecture.md 5.1):
 * a 15-minute JWT access token and an opaque refresh token that rotates on
 * every use. [toString] never prints them, so a stray log line or crash
 * report cannot leak a session.
 */
data class AuthTokens(
    /** The JWT sent as `Authorization: Bearer`. */
    val accessToken: String,
    /** The opaque token exchanged for a new pair; valid once. */
    val refreshToken: String,
) {
    override fun toString(): String = "AuthTokens(accessToken=██, refreshToken=██)"
}

/**
 * TokenStorage is what the API client needs from the session store. On the
 * device it is core:security's TokenStore (Keystore + DataStore). The client
 * keeps no copy of its own, so a logout, a login or a tenant switch made
 * through the store applies to the very next request.
 */
interface TokenStorage {
    /** Returns the current tokens, or null when nobody is signed in. */
    suspend fun read(): AuthTokens?

    /**
     * Atomically replaces the tokens with [replacement] (null signs out), but
     * only while the store still holds [expected]. Returns false and changes
     * nothing when a logout, login or tenant switch got there first, so a
     * refresh that was already on the wire can never undo it.
     */
    suspend fun replace(
        expected: AuthTokens,
        replacement: AuthTokens?,
    ): Boolean
}

/**
 * TokenRefresher exchanges a refresh token for a new pair at the API.
 *
 * Implementations must mark their request with [withoutBearer] and make it in
 * the calling coroutine, and throw [ApiError] when the server refuses or
 * cannot be reached; the client decides from the status whether that ends the
 * session. The client also refuses to refresh inside a refresh, but only as a
 * safety net: a call launched in another scope escapes it and would deadlock.
 */
fun interface TokenRefresher {
    /** Returns the new pair for [refreshToken]; [client] is the API client itself. */
    suspend fun refresh(
        client: HttpClient,
        refreshToken: String,
    ): AuthTokens
}
