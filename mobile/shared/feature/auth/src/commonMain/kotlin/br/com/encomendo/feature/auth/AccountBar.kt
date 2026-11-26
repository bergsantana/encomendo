package br.com.encomendo.feature.auth

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.heightIn
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.role
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import br.com.encomendo.feature.auth.resources.Res
import br.com.encomendo.feature.auth.resources.account_log_out
import br.com.encomendo.feature.auth.resources.account_log_out_failed
import br.com.encomendo.feature.auth.resources.account_logging_out
import br.com.encomendo.feature.auth.resources.account_menu_description
import br.com.encomendo.feature.auth.resources.account_switch_store
import org.jetbrains.compose.resources.stringResource
import org.koin.compose.viewmodel.koinViewModel

/**
 * AccountBarRoute connects [AccountViewModel] to [AccountBar]; [onSwitchStore]
 * opens the store picker, and [navigationIcon] sits before the store's name.
 */
@Composable
fun AccountBarRoute(
    onSwitchStore: () -> Unit,
    modifier: Modifier = Modifier,
    navigationIcon: @Composable () -> Unit = {},
    viewModel: AccountViewModel = koinViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val actions =
        remember(viewModel, onSwitchStore) {
            AccountActions(
                onMenuOpen = viewModel::onMenuOpen,
                onMenuDismiss = viewModel::onMenuDismiss,
                onSwitchStore = {
                    viewModel.onMenuDismiss()
                    onSwitchStore()
                },
                onLogOut = viewModel::onLogOut,
            )
        }
    AccountBar(state = state, actions = actions, modifier = modifier, navigationIcon = navigationIcon)
}

/** AccountActions are what the user can do on [AccountBar]. */
data class AccountActions(
    val onMenuOpen: () -> Unit,
    val onMenuDismiss: () -> Unit,
    val onSwitchStore: () -> Unit,
    val onLogOut: () -> Unit,
)

/**
 * AccountBar tops every management screen: the store's name, and the
 * signed-in person's name, which opens the account menu with Trocar de loja
 * (two stores or more) and Sair, as the
 * foot of the web admin's menu does. [navigationIcon] is the shell's: the
 * button that opens the management menu.
 */
@OptIn(ExperimentalMaterial3Api::class) // TopAppBar
@Composable
fun AccountBar(
    state: AccountUiState,
    actions: AccountActions,
    modifier: Modifier = Modifier,
    navigationIcon: @Composable () -> Unit = {},
) {
    TopAppBar(
        navigationIcon = navigationIcon,
        title = { Text(text = state.storeName.orEmpty(), maxLines = 1, overflow = TextOverflow.Ellipsis) },
        actions = { AccountMenu(state = state, actions = actions) },
        modifier = modifier,
    )
}

@Composable
private fun AccountMenu(
    state: AccountUiState,
    actions: AccountActions,
) {
    val description = stringResource(Res.string.account_menu_description, state.userName)
    Box {
        TextButton(
            onClick = actions.onMenuOpen,
            modifier =
                Modifier.heightIn(min = WORKSHOP_TARGET).semantics {
                    contentDescription = description
                    role = Role.DropdownList
                },
        ) {
            Text(text = state.userName, maxLines = 1, overflow = TextOverflow.Ellipsis)
        }
        DropdownMenu(expanded = state.menuOpen, onDismissRequest = actions.onMenuDismiss) {
            if (state.canSwitchStore) {
                DropdownMenuItem(
                    text = { Text(stringResource(Res.string.account_switch_store)) },
                    onClick = actions.onSwitchStore,
                    enabled = !state.loggingOut,
                    modifier = Modifier.heightIn(min = WORKSHOP_TARGET),
                )
            }
            DropdownMenuItem(
                text = {
                    Text(
                        stringResource(
                            if (state.loggingOut) Res.string.account_logging_out else Res.string.account_log_out,
                        ),
                    )
                },
                onClick = actions.onLogOut,
                enabled = !state.loggingOut,
                modifier = Modifier.heightIn(min = WORKSHOP_TARGET),
            )
            if (state.logOutFailed) {
                Text(
                    text = stringResource(Res.string.account_log_out_failed),
                    color = MaterialTheme.colorScheme.error,
                    style = MaterialTheme.typography.bodyMedium,
                    modifier = Modifier.semantics { liveRegion = LiveRegionMode.Polite },
                )
            }
        }
    }
}

/** The bar tops the workshop screens too, so its targets follow their 56 dp rule (mobile/CLAUDE.md). */
private val WORKSHOP_TARGET = 56.dp
