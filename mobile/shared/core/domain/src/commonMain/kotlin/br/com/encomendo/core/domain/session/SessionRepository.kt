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

    /**
     * Brings the stored session up to date with the server: its store, role
     * and permissions. Offline or on a server failure the session stays as it
     * was; if the server refuses it, it ends (ADR 0012). Never throws: it is a
     * chance to be up to date, not a step anything waits on.
     */
    suspend fun renew()
}
