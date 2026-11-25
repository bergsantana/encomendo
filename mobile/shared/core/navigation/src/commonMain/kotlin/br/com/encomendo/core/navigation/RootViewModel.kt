package br.com.encomendo.core.navigation

import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelStore
import androidx.lifecycle.ViewModelStoreOwner
import androidx.lifecycle.viewModelScope
import br.com.encomendo.core.domain.session.ObserveSignedIn
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.stateIn

/**
 * RootUiState says which graph the app shows. A signed-in or signed-out state
 * carries [Session.screens], where that session's screen ViewModels live; a
 * new session always comes with a new owner, so the two can never disagree.
 */
sealed interface RootUiState {
    /** The stored session is still being read; show nothing rather than flash the login. */
    data object Loading : RootUiState

    /** A known session state, signed in or out. */
    sealed interface Session : RootUiState {
        /** The ViewModels of this session's screens; cleared when the session ends. */
        val screens: ViewModelStoreOwner
    }

    /** Nobody is signed in: the auth graph. */
    data class SignedOut(
        override val screens: ViewModelStoreOwner,
    ) : Session

    /** A business user is signed in: the management graph. */
    data class SignedIn(
        override val screens: ViewModelStoreOwner,
    ) : Session
}

/**
 * RootViewModel picks the auth graph or the management graph from the
 * session, and switches as soon as it starts or ends (a login, "Sair", or a
 * refresh the server refused).
 *
 * It also owns each session's screen ViewModels. They survive rotation and
 * are cleared when the session starts or ends, so the login form never
 * outlives the login and no management screen keeps running under the next
 * user's token.
 */
class RootViewModel(
    observeSignedIn: ObserveSignedIn,
) : ViewModel() {
    // Read and replaced on the main thread only, by the session flow below.
    private var session: SessionScreens? = null

    /** The graph to show. */
    val uiState: StateFlow<RootUiState> =
        observeSignedIn()
            .map(::stateFor)
            .stateIn(viewModelScope, SharingStarted.WhileSubscribed(STOP_TIMEOUT_MILLIS), RootUiState.Loading)

    override fun onCleared() {
        session?.viewModelStore?.clear()
    }

    // The flow restarts after the app sat in the background, re-emitting the same value;
    // only a real change of session may clear the screens.
    private fun stateFor(signedIn: Boolean): RootUiState.Session {
        val screens = session?.takeIf { it.signedIn == signedIn } ?: startSession(signedIn)
        return if (signedIn) RootUiState.SignedIn(screens) else RootUiState.SignedOut(screens)
    }

    private fun startSession(signedIn: Boolean): SessionScreens {
        session?.viewModelStore?.clear()
        return SessionScreens(signedIn).also { session = it }
    }

    private class SessionScreens(
        val signedIn: Boolean,
    ) : ViewModelStoreOwner {
        override val viewModelStore = ViewModelStore()
    }

    private companion object {
        /** Keeps the session flow alive across a configuration change. */
        const val STOP_TIMEOUT_MILLIS = 5_000L
    }
}
