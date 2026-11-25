package br.com.encomendo.core.navigation

import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelStore
import androidx.lifecycle.ViewModelStoreOwner
import androidx.lifecycle.viewModelScope
import br.com.encomendo.core.domain.session.ObserveSignedIn
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.onEach
import kotlinx.coroutines.flow.stateIn

/** RootUiState says which graph the app shows. */
sealed interface RootUiState {
    /** The stored session is still being read; show nothing rather than flash the login. */
    data object Loading : RootUiState

    /** Nobody is signed in: the auth graph. */
    data object SignedOut : RootUiState

    /** A business user is signed in: the management graph. */
    data object SignedIn : RootUiState
}

/**
 * RootViewModel picks the auth graph or the management graph from the
 * session, and switches as soon as it starts or ends (a login, "Sair", or a
 * refresh the server refused).
 *
 * It also owns the ViewModels of the current session's screens
 * ([sessionViewModels]). They survive rotation, and are cleared when the
 * session starts or ends, so the login form never outlives the login and no
 * management screen keeps running under the next user's token.
 */
class RootViewModel(
    observeSignedIn: ObserveSignedIn,
) : ViewModel() {
    // Read and replaced on the main thread only: by the session flow below and by composition.
    private var session = SessionViewModels(signedIn = null)

    /** The graph to show. */
    val uiState: StateFlow<RootUiState> =
        observeSignedIn()
            .onEach(::startSessionIfChanged)
            .map { signedIn -> if (signedIn) RootUiState.SignedIn else RootUiState.SignedOut }
            .stateIn(viewModelScope, SharingStarted.WhileSubscribed(STOP_TIMEOUT_MILLIS), RootUiState.Loading)

    /** Where the screens of the current session keep their ViewModels. */
    val sessionViewModels: ViewModelStoreOwner get() = session

    override fun onCleared() {
        session.viewModelStore.clear()
    }

    // The flow restarts after the app sat in the background, re-emitting the same state;
    // only a real change of session may clear the screens.
    private fun startSessionIfChanged(signedIn: Boolean) {
        if (session.signedIn == signedIn) return
        session.viewModelStore.clear()
        session = SessionViewModels(signedIn)
    }

    private class SessionViewModels(
        val signedIn: Boolean?,
    ) : ViewModelStoreOwner {
        override val viewModelStore = ViewModelStore()
    }

    private companion object {
        /** Keeps the session flow alive across a configuration change. */
        const val STOP_TIMEOUT_MILLIS = 5_000L
    }
}
