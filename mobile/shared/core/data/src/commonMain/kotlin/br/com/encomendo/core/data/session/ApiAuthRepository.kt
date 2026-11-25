package br.com.encomendo.core.data.session

import br.com.encomendo.core.data.failure.asFailures
import br.com.encomendo.core.domain.session.AuthRepository
import br.com.encomendo.core.domain.session.Session
import br.com.encomendo.core.network.auth.AuthApi
import br.com.encomendo.core.security.TokenStore

/**
 * ApiAuthRepository logs in through the `/auth` endpoints and keeps the
 * session in [store], tokens and all, in one write (ADR 0015). Nothing is
 * stored unless the whole login worked.
 */
class ApiAuthRepository(
    private val api: AuthApi,
    private val store: TokenStore,
) : AuthRepository {
    override suspend fun logIn(
        email: String,
        password: String,
    ): Session =
        asFailures {
            val stored = api.logIn(email = email, password = password).toStoredSession()
            store.write(stored)
            stored.session
        }
}
