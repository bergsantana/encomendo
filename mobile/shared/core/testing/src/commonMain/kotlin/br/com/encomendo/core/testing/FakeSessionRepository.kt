package br.com.encomendo.core.testing

import br.com.encomendo.core.domain.session.SessionRepository
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow

/** FakeSessionRepository lets a test sign the user in and out by setting [state]. */
class FakeSessionRepository(
    signedIn: Boolean = false,
) : SessionRepository {
    /** Whether someone is signed in; set it to simulate a login or a logout. */
    val state = MutableStateFlow(signedIn)

    override val signedIn: StateFlow<Boolean> = state
}
