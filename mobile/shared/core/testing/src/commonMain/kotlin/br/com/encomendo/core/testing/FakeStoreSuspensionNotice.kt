package br.com.encomendo.core.testing

import br.com.encomendo.core.domain.session.StoreSuspensionNotice
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

/** FakeStoreSuspensionNotice is a [StoreSuspensionNotice] for tests: [raisedFor] is what the last call left. */
class FakeStoreSuspensionNotice : StoreSuspensionNotice {
    private val raised = MutableStateFlow<String?>(null)

    override val raisedFor: StateFlow<String?> = raised.asStateFlow()

    override fun raise(tenantId: String) {
        raised.value = tenantId
    }

    override fun dismiss() {
        raised.value = null
    }
}
