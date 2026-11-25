package br.com.encomendo.core.network

import io.ktor.client.HttpClient
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import kotlin.coroutines.CoroutineContext

/**
 * BearerRefresh runs at most one token refresh at a time and decides what a
 * failed refresh means (docs/adr/0012-refresh-failure-keeps-offline-sessions.md).
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
     * its 401.
     *
     * The session ends only when the server refuses the refresh token. Any
     * other failure (offline, 429, 5xx) is rethrown as it is, so the caller sees
     * why and the session survives: workshops lose signal often. Nothing is
     * written if a logout, login or tenant switch replaced the tokens meanwhile.
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
                    withContext(RefreshInFlight) { refresher.refresh(client, stored.refreshToken) }
                } catch (error: ApiError) {
                    if (error.status !in SESSION_ENDING_STATUSES) throw error
                    tokens.replace(expected = stored, replacement = null)
                    return@withLock null
                }
            renewed.takeIf { tokens.replace(expected = stored, replacement = it) }
        }

    /** Marks the coroutine of a refresh call, whose own 401 must never start another refresh. */
    private object RefreshInFlight : CoroutineContext.Element, CoroutineContext.Key<RefreshInFlight> {
        override val key: CoroutineContext.Key<*> get() = this
    }

    companion object {
        /** The server looked at the refresh token and said no; anything else may pass. */
        private val SESSION_ENDING_STATUSES = setOf(400, 401, 403)

        /**
         * True inside a refresh call. Ktor's refresh lock is not reentrant, so a
         * 401 on the refresh call itself would wait on it forever.
         */
        suspend fun isRunning(): Boolean = currentCoroutineContext()[RefreshInFlight] != null
    }
}
