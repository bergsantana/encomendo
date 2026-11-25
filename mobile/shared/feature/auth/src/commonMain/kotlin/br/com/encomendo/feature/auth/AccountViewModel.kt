package br.com.encomendo.feature.auth

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.domain.session.LogOut
import br.com.encomendo.core.domain.session.ObserveSession
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/**
 * AccountViewModel backs the management app bar: the signed-in person and
 * store, and Sair. It never navigates: once the session is gone, the root
 * shows the login by itself.
 */
class AccountViewModel(
    observeSession: ObserveSession,
    private val logOut: LogOut,
) : ViewModel() {
    private val menu = MutableStateFlow(AccountUiState())

    /** The bar as the screen shows it. */
    val uiState: StateFlow<AccountUiState> =
        combine(observeSession(), menu) { session, local ->
            local.copy(
                userName = session?.user?.name.orEmpty(),
                storeName = session?.tenant?.name,
                canSwitchStore = session?.memberships.orEmpty().size >= STORES_TO_SWITCH_BETWEEN,
            )
        }.stateIn(viewModelScope, SharingStarted.WhileSubscribed(STOP_AFTER_MILLIS), AccountUiState())

    /** The user opened the account menu. */
    fun onMenuOpen() = menu.update { it.copy(menuOpen = true) }

    /** The menu closed; its error, if any, goes with it. */
    fun onMenuDismiss() = menu.update { it.copy(menuOpen = false, logOutFailed = false) }

    /** The user tapped Sair. Does nothing while a logout runs. */
    fun onLogOut() {
        if (menu.value.loggingOut) return
        menu.update { it.copy(loggingOut = true, logOutFailed = false) }
        viewModelScope.launch {
            try {
                logOut()
                // Stays busy: the root swaps to the login and clears this ViewModel.
            } catch (_: Failure) {
                // Reopened, so the error shows even if the menu was closed meanwhile.
                menu.update { it.copy(loggingOut = false, logOutFailed = true, menuOpen = true) }
            }
        }
    }
}

private const val STORES_TO_SWITCH_BETWEEN = 2

/** Keeps the session flow alive across a configuration change. */
private const val STOP_AFTER_MILLIS = 5_000L
