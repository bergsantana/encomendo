package br.com.encomendo.core.data.members

import br.com.encomendo.core.data.failure.StoreCalls
import br.com.encomendo.core.data.session.InMemoryStoreSuspensionNotice
import br.com.encomendo.core.data.session.SessionKeeper
import br.com.encomendo.core.domain.session.StoreSuspensionNotice
import br.com.encomendo.core.network.SessionTurns
import br.com.encomendo.core.network.auth.AuthApi
import br.com.encomendo.core.security.TokenStore
import io.ktor.client.HttpClient
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob

/**
 * Wires a [StoreCalls] over [client] for the repository tests. [renewals] is a real scope, so a test that
 * needs the renewal done joins its children.
 */
internal fun storeCalls(
    store: TokenStore,
    client: HttpClient,
    turns: SessionTurns,
    notice: StoreSuspensionNotice = InMemoryStoreSuspensionNotice(),
    renewals: CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.Default),
): StoreCalls = StoreCalls(store, SessionKeeper(AuthApi(client), store, turns), notice, renewals)
