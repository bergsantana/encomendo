package br.com.encomendo.core.data.session

import br.com.encomendo.core.domain.session.Session
import br.com.encomendo.core.domain.session.SessionRepository
import br.com.encomendo.core.security.TokenStore
import kotlinx.coroutines.flow.Flow

/** TokenSessionRepository reads the signed-in session from the device's [TokenStore]. */
class TokenSessionRepository(
    tokens: TokenStore,
) : SessionRepository {
    override val session: Flow<Session?> = tokens.session
}
