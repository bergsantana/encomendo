package br.com.encomendo.core.network

import io.ktor.client.HttpClient
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock

/**
 * BearerRefresh runs at most one token refresh at a time and decides what a
 * failed refresh means.
 *
 * Refresh tokens rotate on every use and a reused one revokes its whole family
 * (docs/architecture.md 5.1), so two refreshes racing with the same token would
 * sign the user out. Requests that were rejected with an access token that has
 * since been replaced get the stored tokens back without a second refresh.
 */
internal class BearerRefresh(
    private val tokens: TokenStorage,
    private val refresher: TokenRefresher,
) {
    private val mutex = Mutex()

    /**
     * Returns tokens to retry with, or null when the request should fail with
     * its 401. Clears [tokens] only when the server refuses the refresh token,
     * never when it cannot be reached: workshops lose signal often.
     */
    suspend fun refresh(
        client: HttpClient,
        rejectedAccessToken: String?,
    ): AuthTokens? =
        mutex.withLock {
            val stored = tokens.read() ?: return@withLock null
            if (stored.accessToken != rejectedAccessToken) return@withLock stored
            val renewed =
                try {
                    refresher.refresh(client, stored.refreshToken)
                } catch (error: ApiError) {
                    if (error.status in SESSION_ENDING_STATUSES) tokens.clear()
                    return@withLock null
                }
            tokens.write(renewed)
            renewed
        }

    private companion object {
        /** The server looked at the refresh token and said no; anything else (offline, 429, 5xx) may pass. */
        val SESSION_ENDING_STATUSES = setOf(400, 401, 403)
    }
}
