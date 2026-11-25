package br.com.encomendo.core.network

import io.ktor.client.HttpClient
import kotlinx.io.IOException

/**
 * BearerRefresh refreshes in its [SessionTurns] turn, so no other call spends
 * the refresh token at the same time, and decides what a failed refresh means
 * (docs/adr/0012-refresh-failure-keeps-offline-sessions.md).
 *
 * Refresh tokens rotate on every use and a reused one revokes its whole family
 * (docs/architecture.md 5.1). The server's 2-minute retry window covers an
 * answer lost on the way back, not two refreshes at once: one of them is left
 * holding a superseded token, and its next use signs the user out (ADR 0013).
 * Requests that were rejected with an access token that has since been
 * replaced (by another refresh, or a store switch in its turn) get the stored
 * tokens back without a second refresh.
 */
internal class BearerRefresh<S : BearerCredentials>(
    private val tokens: TokenStorage<S>,
    private val refresher: TokenRefresher<S>,
    private val turns: SessionTurns,
) {
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
        turns.take {
            val stored = tokens.read() ?: return@take null
            if (stored.accessToken != rejectedAccessToken) return@take stored
            val renewed =
                try {
                    refresher.refresh(client, stored)
                } catch (error: ApiError) {
                    if (error.status !in SESSION_ENDING_STATUSES) throw error
                    signOut(stored)
                    return@take null
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

    private companion object {
        /** The server looked at the refresh token and said no; anything else may pass. */
        val SESSION_ENDING_STATUSES = setOf(400, 401, 403)
    }
}
