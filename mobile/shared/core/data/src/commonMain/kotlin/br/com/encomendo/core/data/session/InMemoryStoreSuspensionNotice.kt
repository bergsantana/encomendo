package br.com.encomendo.core.data.session

import br.com.encomendo.core.domain.session.Session
import br.com.encomendo.core.domain.session.StoreSuspensionNotice
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.map

/**
 * InMemoryStoreSuspensionNotice keeps the notice for the life of the process.
 * Without [follow] it never expires on its own, so the app starts it once.
 */
class InMemoryStoreSuspensionNotice : StoreSuspensionNotice {
    private val raised = MutableStateFlow<String?>(null)

    override val raisedFor: StateFlow<String?> = raised.asStateFlow()

    override fun raise(tenantId: String) {
        raised.value = tenantId
    }

    override fun dismiss() {
        raised.value = null
    }

    /**
     * Drops the notice whenever the signed-in user changes, which includes
     * signing out, so another person on the same phone never reads it. A
     * renewal keeps the same user, so it keeps the notice. Runs until
     * cancelled; start it once, in the app's scope.
     */
    suspend fun follow(sessions: Flow<Session?>) {
        sessions.map { it?.user?.id }.distinctUntilChanged().collect { dismiss() }
    }
}
