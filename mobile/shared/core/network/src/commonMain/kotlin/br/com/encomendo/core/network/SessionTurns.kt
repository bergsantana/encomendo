package br.com.encomendo.core.network

import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import kotlin.coroutines.CoroutineContext

/**
 * SessionTurns lets one call at a time spend the refresh token: the API
 * client's own refresh, a store switch, a refresh on app start.
 *
 * Refresh tokens rotate on every use. Two calls presenting the same token
 * leave one of them holding a superseded token, and its next use revokes the
 * whole family (ADR 0010, ADR 0013). Keep one instance per process, shared
 * by [createApiClient] and every such call.
 *
 * Not reentrant: a turn never waits for another. Asking for one inside a turn
 * fails at once instead of waiting forever, and a 401 inside a turn never
 * starts a refresh (see [createApiClient]).
 */
class SessionTurns {
    private val mutex = Mutex()

    /** Runs [block] once no other turn is running, and returns its result. */
    suspend fun <T> take(block: suspend () -> T): T {
        check(!inTurn()) { "a SessionTurns turn never waits for another: it would wait forever" }
        return mutex.withLock { withContext(InTurn) { block() } }
    }

    /** Marks the coroutine of a turn. */
    private object InTurn : CoroutineContext.Element, CoroutineContext.Key<InTurn> {
        override val key: CoroutineContext.Key<*> get() = this
    }

    /** Whether the caller runs inside a turn. */
    companion object {
        /** True inside a turn, where a 401 must fail rather than wait for another turn to refresh. */
        suspend fun inTurn(): Boolean = currentCoroutineContext()[InTurn] != null
    }
}
