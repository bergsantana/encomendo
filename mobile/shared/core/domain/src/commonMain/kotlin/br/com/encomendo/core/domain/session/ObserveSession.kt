package br.com.encomendo.core.domain.session

import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.distinctUntilChanged

/**
 * ObserveSession emits the signed-in session (null when signed out), once per
 * change, so the app reacts only when the session really changes: a login, a
 * logout, a store switch, or a refresh that brought new permissions.
 */
class ObserveSession(
    private val sessions: SessionRepository,
) {
    /** Returns the session, starting with the stored one. */
    operator fun invoke(): Flow<Session?> = sessions.session.distinctUntilChanged()
}
