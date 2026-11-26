package br.com.encomendo.core.domain.session

import kotlinx.coroutines.flow.Flow

/**
 * ObserveStoreSuspension emits the id of the store that a screen inside it
 * found suspended, until the user acts (see [StoreSuspensionNotice]).
 */
class ObserveStoreSuspension(
    private val notice: StoreSuspensionNotice,
) {
    /** Returns the id of the store a call found suspended, or null when there is none. */
    operator fun invoke(): Flow<String?> = notice.raisedFor
}
