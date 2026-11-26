package br.com.encomendo.core.network

import io.ktor.client.HttpClient
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.update
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
    // The scope of each access token handed out lately: a 401 names only the token it was sent with. Keyed
    // by the token's signature, unique and useless on its own, so no usable token stays in memory after Sair.
    private val issuedScopes = MutableStateFlow<Map<String, String>>(emptyMap())

    /** Remembers which user and store [credentials]' access token acts for, each time the client loads it. */
    fun issued(credentials: S) {
        val key = credentials.accessToken.signature()
        issuedScopes.update { scopes ->
            (scopes - key + (key to credentials.scope))
                .entries
                .toList()
                .takeLast(REMEMBERED_TOKENS)
                .associate { it.key to it.value }
        }
    }

    /**
     * Decides whether the request is retried: non-null to retry, null to fail
     * with its 401. Ktor takes the retry's token from a fresh load, which
     * waits until the screens serve the stored scope; this only says whether a
     * retry may happen. It may only in the scope the request was sent with:
     * after a store switch, it would otherwise run in the new store.
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
            if (stored.accessToken != rejectedAccessToken) {
                return@take stored.takeIf { it.scope == issuedScopes.value[rejectedAccessToken?.signature()] }
            }
            val renewed =
                try {
                    refresher.refresh(client, stored)
                } catch (error: ApiError) {
                    if (!error.refusesRefreshToken) throw error
                    signOut(stored)
                    return@take null
                }
            // Kept even when the refresh left the store, since the old token is spent; but then the
            // request, meant for that store, fails instead of running without it.
            val replaced = tokens.replace(expected = stored, replacement = renewed)
            renewed.takeIf { replaced && it.scope == stored.scope }
        }

    private suspend fun signOut(stored: S) {
        try {
            tokens.replace(expected = stored, replacement = null)
        } catch (_: IOException) {
            // The server already refused the token. The request still fails with its own 401
            // instead of a storage error read as "offline", and the next 401 tries again.
        }
    }

    /** The JWT's signature, its last part; the whole value for a token without one, as in tests. */
    private fun String.signature(): String = substringAfterLast('.')

    private companion object {
        /**
         * Enough for every request a burst of screens has in flight. Eviction fails safe: a
         * token forgotten only turns its retry into a 401.
         */
        const val REMEMBERED_TOKENS = 32
    }
}
