package br.com.encomendo.core.domain.session

import kotlinx.coroutines.flow.StateFlow

/**
 * StoreSuspensionNotice carries one fact across the screens' replacement: a
 * store the user was working in turned out to be suspended (ADR 0022). The
 * session then renews without a store and the root swaps the management
 * screens for the store picker, which clears their ViewModels, so the picker
 * cannot hear it from the failure itself. It reads [raisedFor] to say why the
 * user is there, and the notice goes once the user acts or the session ends
 * or changes user.
 *
 * Held in memory only: after a restart the picker's "Suspensa" mark says it.
 */
interface StoreSuspensionNotice {
    /** The id of the store a call found suspended, or null when there is nothing to say. */
    val raisedFor: StateFlow<String?>

    /** A call in the store [tenantId] was refused with `tenant_suspended`. */
    fun raise(tenantId: String)

    /** The picker has said it, or the user moved on. */
    fun dismiss()
}
