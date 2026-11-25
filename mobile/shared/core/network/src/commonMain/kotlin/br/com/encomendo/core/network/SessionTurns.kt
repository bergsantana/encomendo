package br.com.encomendo.core.network

import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import kotlinx.coroutines.withTimeoutOrNull
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
 * It also knows which scope the app's screens serve ([screensServe]), so a
 * store's token goes out only from that store's screens.
 *
 * Not reentrant: a turn never waits for another. Asking for one inside a turn
 * fails at once instead of waiting forever, and a 401 inside a turn never
 * starts a refresh (see [createApiClient]).
 */
class SessionTurns {
    private val mutex = Mutex()
    private val screens = MutableStateFlow<String?>(null)

    /** Runs [block] once no other turn is running, and returns its result. */
    suspend fun <T> take(block: suspend () -> T): T {
        check(!inTurn()) { "a SessionTurns turn never waits for another: it would wait forever" }
        return mutex.withLock { withContext(InTurn) { block() } }
    }

    /**
     * The app has replaced its screens with ones serving [scope] (a
     * `BearerCredentials.scope`), or with signed-out screens (null). Only
     * those screens may send that scope's token. Until the screens catch up
     * with a store switch, requests wait for the token, and the old screens'
     * requests are cancelled with them (ADR 0018).
     */
    fun screensServe(scope: String?) {
        screens.value = scope
    }

    /** The scope the screens serve now; null while signed out or not known yet, when any token may go. */
    internal fun screensScope(): String? = screens.value

    /**
     * Suspends until the screens serve something other than [scope], or for
     * [RECHECK_MILLIS] at most: the stored session may have come back to the
     * screens' own scope (a switch away and back), and nothing would change
     * them. The caller reads again either way.
     */
    internal suspend fun awaitScreensOtherThan(scope: String?) {
        withTimeoutOrNull(RECHECK_MILLIS) { screens.first { it != scope } }
    }

    /** Marks the coroutine of a turn. */
    private object InTurn : CoroutineContext.Element, CoroutineContext.Key<InTurn> {
        override val key: CoroutineContext.Key<*> get() = this
    }

    /** Whether the caller runs inside a turn, and how often a waiting token looks again. */
    companion object {
        /** How long a request waits for the screens before it reads the stored session again. */
        internal const val RECHECK_MILLIS = 1_000L

        /** True inside a turn, where a 401 must fail rather than wait for another turn to refresh. */
        suspend fun inTurn(): Boolean = currentCoroutineContext()[InTurn] != null
    }
}
