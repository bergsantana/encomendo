package br.com.encomendo.core.network

import io.ktor.client.HttpClient
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import kotlinx.io.IOException
import kotlin.coroutines.CoroutineContext

/**
 * BearerRefresh runs at most one token refresh at a time and decides what a
 * failed refresh means (docs/adr/0012-refresh-failure-keeps-offline-sessions.md).
 *
 * Refresh tokens rotate on every use and a reused one revokes its whole family
 * (docs/architecture.md 5.1). The server's 2-minute retry window covers an
 * answer lost on the way back, not two refreshes at once: one of them is left
 * holding a superseded token, and its next use signs the user out (ADR 0013).
 * Requests that were rejected with an access token that has since been
 * replaced get the stored tokens back without a second refresh.
 */
internal class BearerRefresh<S : BearerCredentials>(
    private val tokens: TokenStorage<S>,
    private val refresher: TokenRefresher<S>,
) {
    private val mutex = Mutex()

    /**
     * Returns the session to retry with, or null when the request should fail
     * with its 401.
     *
     * The session ends only when the server refuses the refresh token. Any
     * other failure (offline, 429, 5xx) is rethrown as it is, so the caller sees
     * why and the session survives: workshops lose signal often. Nothing is
     * written if a logout, login or tenant switch replaced the tokens meanwhile.
     */
    suspend fun refresh(
        client: HttpClient,
        rejectedAccessToken: String?,
    ): S? =
        mutex.withLock {
            val stored = tokens.read() ?: return@withLock null
            if (stored.accessToken != rejectedAccessToken) return@withLock stored
            val renewed =
                try {
                    withContext(RefreshInFlight) { refresher.refresh(client, stored) }
                } catch (error: ApiError) {
                    if (error.status !in SESSION_ENDING_STATUSES) throw error
                    signOut(stored)
                    return@withLock null
                }
            renewed.takeIf { tokens.replace(expected = stored, replacement = it) }
        }

    private suspend fun signOut(stored: S) {
        try {
            tokens.replace(expected = stored, replacement = null)
        } catch (_: IOException) {
            // The server already refused the token. The request still fails with its own 401
            // instead of a storage error read as "offline", and the next 401 tries again.
        }
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
