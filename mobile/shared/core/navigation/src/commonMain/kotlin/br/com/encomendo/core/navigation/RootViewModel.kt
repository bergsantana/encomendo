package br.com.encomendo.core.navigation

import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelStore
import androidx.lifecycle.ViewModelStoreOwner
import androidx.lifecycle.viewModelScope
import br.com.encomendo.core.domain.session.GrantedPermissions
import br.com.encomendo.core.domain.session.ObserveSession
import br.com.encomendo.core.domain.session.Session
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.stateIn

/**
 * RootUiState says which graph the app shows. A signed-in or signed-out state
 * carries [RootUiState.Ready.screens], where that session's screen ViewModels live; a
 * new session always comes with a new owner, so the two can never disagree.
 */
sealed interface RootUiState {
    /** The stored session is still being read; show nothing rather than flash the login. */
    data object Loading : RootUiState

    /** A known session state, signed in or out. */
    sealed interface Ready : RootUiState {
        /** The ViewModels of this session's screens; cleared when the session ends or changes user or store. */
        val screens: ViewModelStoreOwner
    }

    /** Nobody is signed in: the auth graph. */
    data class SignedOut(
        override val screens: ViewModelStoreOwner,
    ) : Ready

    /**
     * Signed in, but no store is selected: right after sign-up, or with
     * several stores and none picked. The store picker, since every store
     * route would answer `403 tenant_required` (ADR 0010).
     */
    data class ChoosingStore(
        override val screens: ViewModelStoreOwner,
    ) : Ready

    /** A business user is signed in to a store: the management graph, showing what [permissions] allow. */
    data class SignedIn(
        override val screens: ViewModelStoreOwner,
        /** What the session's role allows in the selected store. */
        val permissions: GrantedPermissions,
    ) : Ready
}

/**
 * RootViewModel picks the auth graph, the store picker or the management
 * graph from the session, and switches as soon as it starts, ends or moves
 * (a login, "Sair", a store switch, or a refresh the server refused).
 *
 * It also owns each session's screen ViewModels. They survive rotation and
 * are cleared when the session starts or ends, or moves to another store, so
 * the login form never outlives the login, no management screen keeps running
 * under the next user's token, and none keeps showing the previous store. A
 * refresh of the same user and store keeps them.
 */
class RootViewModel(
    observeSession: ObserveSession,
) : ViewModel() {
    // Read and replaced on the main thread only, by the session flow below.
    private var current: SessionScreens? = null

    /** The graph to show. */
    val uiState: StateFlow<RootUiState> =
        observeSession()
            .map(::stateFor)
            .stateIn(viewModelScope, SharingStarted.WhileSubscribed(STOP_TIMEOUT_MILLIS), RootUiState.Loading)

    override fun onCleared() {
        current?.viewModelStore?.clear()
    }

    // The flow restarts after the app sat in the background, re-emitting the same value,
    // and a refresh re-emits the same user and store; only a new user or store clears the screens.
    private fun stateFor(session: Session?): RootUiState.Ready {
        val identity = session?.let { Identity(userId = it.user.id, tenantId = it.tenant?.id) }
        val screens = current?.takeIf { it.identity == identity } ?: startSession(identity)
        return when {
            session == null -> RootUiState.SignedOut(screens)
            session.tenant == null -> RootUiState.ChoosingStore(screens)
            else -> RootUiState.SignedIn(screens, session.permissions)
        }
    }

    private fun startSession(identity: Identity?): SessionScreens {
        current?.viewModelStore?.clear()
        return SessionScreens(identity).also { current = it }
    }

    /** Who the screens work for: a user in a store, or nobody (null) while signed out. */
    private data class Identity(
        val userId: String,
        val tenantId: String?,
    )

    private class SessionScreens(
        val identity: Identity?,
    ) : ViewModelStoreOwner {
        override val viewModelStore = ViewModelStore()
    }

    private companion object {
        /** Keeps the session flow alive across a configuration change. */
        const val STOP_TIMEOUT_MILLIS = 5_000L
    }
}
