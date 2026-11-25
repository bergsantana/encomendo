package br.com.encomendo.core.testing

import br.com.encomendo.core.domain.session.Session
import br.com.encomendo.core.domain.session.SessionRepository
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow

/** FakeSessionRepository lets a test sign the user in and out, or refresh the session, by setting [state]. */
class FakeSessionRepository(
    session: Session? = null,
) : SessionRepository {
    /** The signed-in session, or null; set it to simulate a login, refresh, store switch or logout. */
    val state = MutableStateFlow(session)

    override val session: StateFlow<Session?> = state
}
