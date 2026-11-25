package br.com.encomendo.core.domain.session

import kotlinx.coroutines.flow.Flow

/**
 * SessionRepository tells who is signed in on this device. The app opens the
 * auth graph or the management graph from it, and shows what the session's
 * permissions allow.
 */
interface SessionRepository {
    /**
     * Emits the stored session first (null when nobody is signed in), then every
     * change: a login, a refresh, a store switch, a logout or a refused refresh.
     */
    val session: Flow<Session?>
}
