package br.com.encomendo.core.network

import io.ktor.client.HttpClient

/**
 * BearerCredentials is what the API client reads from a stored session: the
 * token pair (docs/architecture.md 5.1), a 15-minute JWT access token and an
 * opaque refresh token that rotates on every use. Implementations must never
 * print the tokens in toString, so a log line or crash report cannot leak a
 * session.
 */
interface BearerCredentials {
    /** The JWT sent as `Authorization: Bearer`. */
    val accessToken: String

    /** The opaque token exchanged for a new session; valid once. */
    val refreshToken: String
}

/**
 * TokenStorage is what the API client needs from the session store. On the
 * device it is core:security's TokenStore (Keystore + DataStore). The client
 * keeps no copy of its own, so a logout, a login or a tenant switch written
 * to the store applies to the very next request. Writing to the store does not
 * serialize the API call that produced it: see createApiClient for calls that
 * spend the refresh token.
 *
 * [S] is whatever the store keeps with the tokens (the session's store and
 * permissions on the device); the client reads only the tokens and hands [S]
 * back unchanged.
 */
interface TokenStorage<S : BearerCredentials> {
    /** Returns the current session, or null when nobody is signed in. */
    suspend fun read(): S?

    /**
     * Atomically replaces the session with [replacement] (null signs out), but
     * only while the store still holds [expected]. Returns false and changes
     * nothing when a logout, login or tenant switch got there first, so a
     * refresh that was already on the wire can never undo it.
     */
    suspend fun replace(
        expected: S,
        replacement: S?,
    ): Boolean
}

/**
 * TokenRefresher exchanges the current session's refresh token for a new
 * session at the API.
 *
 * Implementations must mark their request with [withoutBearer] and make it in
 * the calling coroutine, and throw [ApiError] when the server refuses or
 * cannot be reached; the client decides from the status whether that ends the
 * session. The client also refuses to refresh inside a refresh, but only as a
 * safety net: a call launched in another scope escapes it and would deadlock.
 */
fun interface TokenRefresher<S : BearerCredentials> {
    /** Returns the session that replaces [current]; [client] is the API client itself. */
    suspend fun refresh(
        client: HttpClient,
        current: S,
    ): S
}
