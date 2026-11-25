package br.com.encomendo.core.domain.session

import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.distinctUntilChanged

/**
 * ObserveSignedIn emits whether someone is signed in, once per change, so
 * the app switches between the auth and management graphs only when the
 * session really starts or ends.
 */
class ObserveSignedIn(
    private val sessions: SessionRepository,
) {
    /** Returns the sign-in state, starting with the stored one. */
    operator fun invoke(): Flow<Boolean> = sessions.signedIn.distinctUntilChanged()
}
