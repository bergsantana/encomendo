package br.com.encomendo.core.data.session

import br.com.encomendo.core.domain.session.SessionRepository
import br.com.encomendo.core.security.TokenStore
import kotlinx.coroutines.flow.Flow

/** TokenSessionRepository reads the sign-in state from the device's [TokenStore]. */
class TokenSessionRepository(
    tokens: TokenStore,
) : SessionRepository {
    override val signedIn: Flow<Boolean> = tokens.signedIn
}
