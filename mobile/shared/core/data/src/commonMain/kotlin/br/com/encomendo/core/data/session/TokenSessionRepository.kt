package br.com.encomendo.core.data.session

import br.com.encomendo.core.domain.session.Session
import br.com.encomendo.core.domain.session.SessionRepository
import br.com.encomendo.core.security.TokenStore
import kotlinx.coroutines.flow.Flow

/**
 * TokenSessionRepository reads the signed-in session from the device's
 * [TokenStore], and renews it through [keeper], in the turn every call that
 * spends the refresh token takes.
 */
class TokenSessionRepository(
    tokens: TokenStore,
    private val keeper: SessionKeeper,
) : SessionRepository {
    override val session: Flow<Session?> = tokens.session

    override suspend fun renew() = keeper.renew()
}
