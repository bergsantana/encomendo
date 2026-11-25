package br.com.encomendo.core.data.session

import br.com.encomendo.core.data.failure.asFailures
import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.domain.session.Session
import br.com.encomendo.core.domain.session.TenancyRepository
import br.com.encomendo.core.domain.session.TenantSummary
import br.com.encomendo.core.network.ApiError
import br.com.encomendo.core.network.SessionTurns
import br.com.encomendo.core.network.auth.AuthApi
import br.com.encomendo.core.network.tenancy.TenancyApi
import br.com.encomendo.core.security.StoredSession
import br.com.encomendo.core.security.TokenStore
import kotlinx.coroutines.NonCancellable
import kotlinx.coroutines.withContext
import kotlinx.io.IOException

/**
 * ApiTenancyRepository creates stores and switches the session between them.
 * A switch spends the refresh token, so it runs in its [turns] turn, the same
 * one the API client refreshes in (ADR 0010, ADR 0013), and keeps the new
 * session in [store] with its tokens (ADR 0015). The new store's token goes
 * out only once the app's screens serve it (ADR 0018).
 */
class ApiTenancyRepository(
    private val auth: AuthApi,
    private val tenancy: TenancyApi,
    private val store: TokenStore,
    private val turns: SessionTurns,
) : TenancyRepository {
    override suspend fun switchTo(tenantId: String): Session =
        asFailures {
            turns.take {
                val current = store.read() ?: throw signedOutMeanwhile()
                // Not cancellable once sent: the server may rotate the token even if the screen that
                // asked is gone, and dropping the answer would leave a spent token on the device.
                withContext(NonCancellable) { switch(current, tenantId) }
            }
        }

    private suspend fun switch(
        current: StoredSession,
        tenantId: String,
    ): Session {
        val switched = rotate(current, tenantId)
        keep(current, switched)
        return switched.session
    }

    private suspend fun rotate(
        current: StoredSession,
        tenantId: String,
    ): StoredSession =
        try {
            auth.switchTenant(refreshToken = current.tokens.refreshToken, tenantId = tenantId).toStoredSession()
        } catch (error: ApiError) {
            // 400 and 403 are checked before anything rotates (ADR 0013); a 401 is the refresh
            // token itself refused, as on /auth/refresh, and ends the session.
            if (error.status == UNAUTHORIZED) forget(current)
            throw error
        }

    /**
     * Stores [switched] in place of [current]. The new store's token then
     * waits until the root has replaced the old store's screens (ADR 0018).
     */
    private suspend fun keep(
        current: StoredSession,
        switched: StoredSession,
    ) {
        if (!store.replace(expected = current, replacement = switched)) throw signedOutMeanwhile()
    }

    override suspend fun createStore(
        name: String,
        slug: String,
    ): TenantSummary =
        asFailures {
            val tenant = tenancy.createTenant(name = name, slug = slug)
            TenantSummary(id = tenant.id, name = tenant.name, slug = tenant.slug)
        }

    private suspend fun forget(current: StoredSession) {
        try {
            store.replace(expected = current, replacement = null)
        } catch (_: IOException) {
            // The request still fails with its 401, and the next refusal tries again.
        }
    }

    /** A logout won the race; the screen that asked is already on its way out. */
    private fun signedOutMeanwhile() = Failure.Unexpected(status = 0, code = "signed_out")

    private companion object {
        const val UNAUTHORIZED = 401
    }
}
