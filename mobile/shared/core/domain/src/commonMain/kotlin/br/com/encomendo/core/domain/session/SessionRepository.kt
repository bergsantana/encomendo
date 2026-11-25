package br.com.encomendo.core.domain.session

import kotlinx.coroutines.flow.Flow

/**
 * SessionRepository tells whether a business user is signed in on this
 * device. The app opens the auth graph or the management graph from it.
 */
interface SessionRepository {
    /** Emits the current state first, then every change: login, logout or a refused refresh. */
    val signedIn: Flow<Boolean>
}
