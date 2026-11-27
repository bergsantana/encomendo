package br.com.encomendo.core.data.session

import br.com.encomendo.core.data.failure.asFailures
import br.com.encomendo.core.domain.session.AuthRepository
import br.com.encomendo.core.domain.session.Session
import br.com.encomendo.core.network.ApiError
import br.com.encomendo.core.network.auth.AuthApi
import br.com.encomendo.core.security.SessionTokens
import br.com.encomendo.core.security.TokenStore
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.NonCancellable
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/**
 * ApiAuthRepository logs in, signs up and logs out through the `/auth` endpoints and keeps
 * the session in [store], tokens and all, in one write (ADR 0015). Nothing
 * is stored unless the whole login worked. A login asks for the store this
 * device used last ([lastStore]). Logging out also empties what [deviceData] cached.
 *
 * @param appScope lives as long as the app, so the server hears about a
 *   logout even after the screen that asked for it is gone.
 */
class ApiAuthRepository(
    private val api: AuthApi,
    private val store: TokenStore,
    private val appScope: CoroutineScope,
    private val lastStore: LastStore,
    private val deviceData: DeviceDataCleaner,
) : AuthRepository {
    override suspend fun logIn(
        email: String,
        password: String,
    ): Session =
        asFailures {
            val stored = api.logIn(email = email, password = password, tenantId = lastStore.read()).toStoredSession()
            store.write(stored)
            stored.session
        }

    override suspend fun signUp(
        name: String,
        email: String,
        password: String,
    ): Session =
        asFailures {
            // Not cancellable: once sent the account may exist, and dropping its session would leave the
            // user unable to sign up again with that email.
            withContext(NonCancellable) {
                val stored = api.signUp(name = name, email = email, password = password).toStoredSession()
                store.write(stored)
                stored.session
            }
        }

    override suspend fun logOut() =
        asFailures {
            // The device forgets first, so leaving never waits on a weak signal (ADR 0016). Not
            // cancellable: the root clears the screen that asked as soon as it sees the session end,
            // before the clear returns here, and the revoke must still be sent.
            withContext(NonCancellable) {
                val leaving = forget()
                // Only once the session is gone: a logout the device could not save leaves the screen as it was.
                deviceData.clear()
                if (leaving != null) appScope.launch { revoke(leaving) }
            }
        }

    /**
     * Clears the stored session and returns the tokens it held, or null when
     * nobody was signed in. A compare-and-swap, so the tokens revoked are the
     * ones cleared even when a refresh lands in between. If the clear fails,
     * nothing was revoked either and the session on screen stays usable.
     */
    private tailrec suspend fun forget(): SessionTokens? {
        val stored = store.read() ?: return null
        return if (store.replace(expected = stored, replacement = null)) stored.tokens else forget()
    }

    private suspend fun revoke(tokens: SessionTokens) {
        try {
            api.logOut(accessToken = tokens.accessToken, refreshToken = tokens.refreshToken)
        } catch (_: ApiError) {
            // Offline, refused or a server failure: the device holds no token any more (ADR 0016).
        }
    }
}
