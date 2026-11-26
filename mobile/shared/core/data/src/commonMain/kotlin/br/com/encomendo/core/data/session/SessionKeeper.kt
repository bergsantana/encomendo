package br.com.encomendo.core.data.session

import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.network.ApiError
import br.com.encomendo.core.network.SessionTurns
import br.com.encomendo.core.network.auth.AuthApi
import br.com.encomendo.core.network.refusesRefreshToken
import br.com.encomendo.core.security.StoredSession
import br.com.encomendo.core.security.TokenStore
import kotlinx.coroutines.NonCancellable
import kotlinx.coroutines.delay
import kotlinx.coroutines.withContext
import kotlinx.io.IOException

/**
 * SessionKeeper makes the calls that spend the refresh token outside the API
 * client's own 401 refresh: a renewal (on app start, or after a refused
 * switch) and a store switch. Each runs in its [turns] turn, the one the
 * client refreshes in (ADR 0010, ADR 0013), and is not cancellable once sent:
 * the server may rotate the token even if the caller is gone, and dropping
 * the answer would leave a spent token on the device. The new session is kept
 * in [store] with its tokens, with a compare-and-swap, so a logout or login
 * that came first stays (ADR 0015).
 */
class SessionKeeper(
    private val auth: AuthApi,
    private val store: TokenStore,
    private val turns: SessionTurns,
) {
    /**
     * Refreshes the stored session: new tokens, and the store, role and
     * permissions the server has now. Offline, 429 and 5xx keep the session as
     * it was; a refused refresh token (400, 401, 403) ends it (ADR 0012).
     *
     * When the outcome is unknown (no answer, a 5xx, an answer the app could
     * not read or store) the server may have rotated the token anyway, so it
     * tries again a couple of times, well inside the 2 minutes in which
     * presenting the old token is a retry, not reuse (ADR 0013). Waiting for
     * the next 401 instead, up to 15 minutes later, would revoke the family.
     *
     * Never throws, except for cancellation before a send: it is a chance to
     * be up to date, not a step anything waits on.
     */
    suspend fun renew() {
        for (wait in listOf(0L) + RETRY_AFTER_MILLIS) {
            delay(wait)
            if (renewOnce() == Outcome.Settled) return
        }
    }

    /**
     * Spends the refresh token for a session in [tenantId] and keeps it.
     * Throws [ApiError] when the server refuses (a 401 also ends the session,
     * since the refresh token itself was refused), and `Failure.Unexpected`
     * when a logout came first.
     */
    suspend fun switchTo(tenantId: String): StoredSession =
        turns.take {
            val current = store.read() ?: throw signedOutMeanwhile()
            withContext(NonCancellable) { switch(current, tenantId) }
        }

    private suspend fun renewOnce(): Outcome =
        try {
            turns.take {
                val current = store.read() ?: return@take Outcome.Settled
                withContext(NonCancellable) { renew(current) }
            }
        } catch (_: IOException) {
            // The device's store failed, maybe after the server rotated: try again.
            Outcome.Unknown
        }

    private suspend fun renew(current: StoredSession): Outcome {
        val renewed =
            try {
                auth.refresh(current.tokens.refreshToken).toStoredSession()
            } catch (error: ApiError) {
                return when {
                    error.refusesRefreshToken -> Outcome.Settled.also { forget(current) }
                    error.mayHaveRotated() -> Outcome.Unknown
                    else -> Outcome.Settled // a 429 or another answer: the server said what it did
                }
            }
        store.replace(expected = current, replacement = renewed)
        return Outcome.Settled
    }

    /** No answer, a 5xx, or a 2xx the app could not read: the token may have been spent. */
    private fun ApiError.mayHaveRotated(): Boolean = status == 0 || status >= FIRST_SERVER_ERROR || status in SUCCESS

    /** Whether a renewal knows how it ended, or must try again. */
    private enum class Outcome { Settled, Unknown }

    private suspend fun switch(
        current: StoredSession,
        tenantId: String,
    ): StoredSession {
        val switched =
            try {
                auth.switchTenant(refreshToken = current.tokens.refreshToken, tenantId = tenantId).toStoredSession()
            } catch (error: ApiError) {
                // 400 and 403 are checked before anything rotates (ADR 0013); a 401 is the refresh
                // token itself refused, as on /auth/refresh, and ends the session.
                if (error.status == UNAUTHORIZED) forget(current)
                throw error
            }
        // The new store's token then waits until the root has replaced the old store's screens (ADR 0018).
        if (!store.replace(expected = current, replacement = switched)) throw signedOutMeanwhile()
        return switched
    }

    private suspend fun forget(current: StoredSession) {
        try {
            store.replace(expected = current, replacement = null)
        } catch (_: IOException) {
            // The call still fails with its refusal, and the next one tries again.
        }
    }

    /** A logout won the race; the screen that asked is already on its way out. */
    private fun signedOutMeanwhile() = Failure.Unexpected(status = 0, code = "signed_out")

    private companion object {
        const val UNAUTHORIZED = 401
        const val FIRST_SERVER_ERROR = 500
        val SUCCESS = 200..299

        /** Waits before trying a renewal again: at 15 s and 45 s, inside ADR 0013's 2-minute window. */
        val RETRY_AFTER_MILLIS = listOf(15_000L, 30_000L)
    }
}
