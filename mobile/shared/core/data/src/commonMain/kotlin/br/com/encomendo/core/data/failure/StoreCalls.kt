package br.com.encomendo.core.data.failure

import br.com.encomendo.core.data.session.SessionKeeper
import br.com.encomendo.core.domain.session.StoreSuspensionNotice
import br.com.encomendo.core.network.ApiError
import br.com.encomendo.core.network.ApiErrorCode
import br.com.encomendo.core.security.TokenStore
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.launch
import kotlinx.io.IOException

/**
 * StoreCalls runs the API calls made inside the selected store (every admin
 * route) and turns their errors into `Failure`s, as [asFailures] does. On top
 * of that, a `tenant_suspended` answer means the platform suspended the store
 * (ADR 0022): it raises [notice] and starts a renewal of the session, which
 * comes back without a store, so the root opens the store picker, as after a
 * refused switch (ADR 0013).
 *
 * Every repository of a store's data must make its calls through [inStore],
 * or its screens would learn of a suspension only as a plain failure and
 * stay in the store.
 *
 * The renewal runs in [renewals], not in the caller: the caller gets its
 * failure at once instead of waiting for the renewal's retries, and the
 * renewal still finishes if the root clears the caller's screen meanwhile.
 * It replaces the session only for the store's screens to be swapped, which
 * keeps requests in the store they were made for (ADR 0018).
 */
class StoreCalls(
    private val tokens: TokenStore,
    private val keeper: SessionKeeper,
    private val notice: StoreSuspensionNotice,
    private val renewals: CoroutineScope,
) {
    /** Runs [call], throwing only `Failure`s. */
    internal suspend fun <T> inStore(call: suspend () -> T): T =
        asFailures {
            try {
                call()
            } catch (error: ApiError) {
                if (error.code == ApiErrorCode.TenantSuspended) suspended()
                throw error
            }
        }

    private suspend fun suspended() {
        // Raised before the renewal: the renewal is what makes the root replace the asking screen.
        currentStoreId()?.let(notice::raise)
        renewals.launch { keeper.renew() }
    }

    // An unreadable session file must not hide the suspension behind a storage failure.
    private suspend fun currentStoreId(): String? =
        try {
            tokens
                .read()
                ?.session
                ?.tenant
                ?.id
        } catch (_: IOException) {
            null
        }
}
