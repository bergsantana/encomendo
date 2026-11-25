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
 * TokenStorage is where the API client reads and replaces the session's
 * tokens. On the device it is backed by core:security's TokenStore (Keystore
 * + DataStore); the client keeps no copy of its own, so clearing the storage
 * signs out every later request at once.
 */
interface TokenStorage {
    /** Returns the current tokens, or null when nobody is signed in. */
    suspend fun read(): AuthTokens?

    /** Replaces the current tokens, after a login, a refresh or a tenant switch. */
    suspend fun write(tokens: AuthTokens)

    /** Forgets the tokens: the user is signed out. */
    suspend fun clear()
}

/**
 * TokenRefresher exchanges a refresh token for a new pair at the API.
 *
 * Implementations must mark their request with [withoutBearer] and throw
 * [ApiError] when the server refuses or cannot be reached; the client decides
 * from the status whether that ends the session.
 */
fun interface TokenRefresher {
    /** Returns the new pair for [refreshToken]; [client] is the API client itself. */
    suspend fun refresh(
        client: HttpClient,
        refreshToken: String,
    ): AuthTokens
}
