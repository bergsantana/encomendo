package br.com.encomendo.core.navigation

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import br.com.encomendo.core.domain.session.ObserveSignedIn
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.map
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
 */
class RootViewModel(
    observeSignedIn: ObserveSignedIn,
) : ViewModel() {
    /** The graph to show. */
    val uiState: StateFlow<RootUiState> =
        observeSignedIn()
            .map { signedIn -> if (signedIn) RootUiState.SignedIn else RootUiState.SignedOut }
            .stateIn(viewModelScope, SharingStarted.WhileSubscribed(STOP_TIMEOUT_MILLIS), RootUiState.Loading)

    private companion object {
        /** Keeps the session flow alive across a configuration change. */
        const val STOP_TIMEOUT_MILLIS = 5_000L
    }
}
