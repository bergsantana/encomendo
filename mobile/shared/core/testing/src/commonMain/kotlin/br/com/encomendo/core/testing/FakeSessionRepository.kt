package br.com.encomendo.core.testing

import br.com.encomendo.core.domain.session.Session
import br.com.encomendo.core.domain.session.SessionRepository
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow

/**
 * FakeSessionRepository lets a test sign the user in and out, or refresh the
 * session, by setting [state], and counts the [renewals] asked of it.
 */
class FakeSessionRepository(
    session: Session? = null,
) : SessionRepository {
    /** The signed-in session, or null; set it to simulate a login, refresh, store switch or logout. */
    val state = MutableStateFlow(session)

    override val session: StateFlow<Session?> = state

    /** How many times the session was asked to renew. */
    var renewals: Int = 0
        private set

    /** Set to make every renewal wait for it, as a renewal on a weak signal does. */
    var renewalGate: CompletableDeferred<Unit>? = null

    override suspend fun renew() {
        renewals++
        renewalGate?.await()
    }
}
