package br.com.encomendo.feature.auth

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusDirection
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import br.com.encomendo.feature.auth.resources.Res
import br.com.encomendo.feature.auth.resources.account_log_out
import br.com.encomendo.feature.auth.resources.account_log_out_failed
import br.com.encomendo.feature.auth.resources.account_logging_out
import br.com.encomendo.feature.auth.resources.login_error_offline
import br.com.encomendo.feature.auth.resources.login_error_server
import br.com.encomendo.feature.auth.resources.picker_choose_title
import br.com.encomendo.feature.auth.resources.picker_create
import br.com.encomendo.feature.auth.resources.picker_create_another
import br.com.encomendo.feature.auth.resources.picker_create_title
import br.com.encomendo.feature.auth.resources.picker_creating
import br.com.encomendo.feature.auth.resources.picker_error_check_fields
import br.com.encomendo.feature.auth.resources.picker_error_not_member
import br.com.encomendo.feature.auth.resources.picker_error_unexpected
import br.com.encomendo.feature.auth.resources.picker_link_hint
import br.com.encomendo.feature.auth.resources.picker_link_rule
import br.com.encomendo.feature.auth.resources.picker_link_taken
import br.com.encomendo.feature.auth.resources.picker_opening
import br.com.encomendo.feature.auth.resources.picker_store_address
import br.com.encomendo.feature.auth.resources.picker_store_link
import br.com.encomendo.feature.auth.resources.picker_store_name
import org.jetbrains.compose.resources.stringResource
import org.koin.compose.viewmodel.koinViewModel

/**
 * StorePickerRoute connects [StorePickerViewModel] to [StorePickerScreen].
 * [onClose] leaves the picker when the store chosen is the one already open.
 */
@Composable
fun StorePickerRoute(
    modifier: Modifier = Modifier,
    onClose: () -> Unit = {},
    viewModel: StorePickerViewModel = koinViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val close by rememberUpdatedState(onClose)
    LaunchedEffect(viewModel) {
        viewModel.effects.collect { effect ->
            when (effect) {
                PickerEffect.Close -> close()
            }
        }
    }
    val actions =
        remember(viewModel) {
            StorePickerActions(
                onChoose = viewModel::onChoose,
                onNameChange = viewModel::onNameChange,
                onSlugChange = viewModel::onSlugChange,
                onCreate = viewModel::onCreate,
                onLogOut = viewModel::onLogOut,
            )
        }
    StorePickerScreen(state = state, actions = actions, modifier = modifier)
}

/** StorePickerActions are what the user can do on [StorePickerScreen]. */
data class StorePickerActions(
    val onChoose: (tenantId: String) -> Unit,
    val onNameChange: (String) -> Unit,
    val onSlugChange: (String) -> Unit,
    val onCreate: () -> Unit,
    val onLogOut: () -> Unit,
)

/**
 * StorePickerScreen is where a member without a store selected lands: right
 * after sign-up (create the first store), or with several stores (choose
 * one, or create another), as on the web admin. Also reached from "Trocar de
 * loja". Stateless; Sair is always there, since it may be the only way out.
 */
@Composable
fun StorePickerScreen(
    state: StorePickerUiState,
    actions: StorePickerActions,
    modifier: Modifier = Modifier,
) {
    // Nothing until the session is read: the title depends on whether there are stores.
    val stores = state.stores ?: return
    val hasStores = stores.isNotEmpty()
    Column(
        modifier = modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(24.dp),
        verticalArrangement = Arrangement.spacedBy(16.dp),
    ) {
        Heading(
            if (hasStores) {
                stringResource(
                    Res.string.picker_choose_title,
                )
            } else {
                stringResource(Res.string.picker_create_title)
            },
        )
        state.error?.let { Announced(it.message(), color = MaterialTheme.colorScheme.error) }
        stores.forEach { store ->
            StoreButton(store = store, opening = state.opening == store.id, enabled = !state.busy, onClick = {
                actions.onChoose(store.id)
            })
        }
        if (hasStores) Heading(stringResource(Res.string.picker_create_another), MaterialTheme.typography.titleMedium)
        CreateStoreForm(state = state, actions = actions, primary = !hasStores)
        TextButton(onClick = actions.onLogOut, enabled = !state.busy, modifier = Modifier.heightIn(min = TARGET)) {
            Text(stringResource(if (state.loggingOut) Res.string.account_logging_out else Res.string.account_log_out))
        }
        if (state.logOutFailed) {
            Announced(
                stringResource(Res.string.account_log_out_failed),
                MaterialTheme.colorScheme.error,
            )
        }
    }
}

@Composable
private fun StoreButton(
    store: StoreChoice,
    opening: Boolean,
    enabled: Boolean,
    onClick: () -> Unit,
) {
    val openingLabel = stringResource(Res.string.picker_opening)
    OutlinedButton(
        onClick = onClick,
        enabled = enabled,
        modifier =
            Modifier.fillMaxWidth().heightIn(min = TARGET).semantics {
                if (opening) contentDescription = openingLabel
            },
    ) {
        if (opening) {
            CircularProgressIndicator(modifier = Modifier.size(24.dp), strokeWidth = 2.dp)
        } else {
            Column(modifier = Modifier.fillMaxWidth()) {
                Text(text = store.name, style = MaterialTheme.typography.titleMedium)
                Text(
                    text = stringResource(Res.string.picker_store_address, store.slug),
                    style = MaterialTheme.typography.bodyMedium,
                )
            }
        }
    }
}

@Composable
private fun CreateStoreForm(
    state: StorePickerUiState,
    actions: StorePickerActions,
    primary: Boolean,
) {
    val focus = LocalFocusManager.current
    OutlinedTextField(
        value = state.name,
        onValueChange = actions.onNameChange,
        label = { Text(stringResource(Res.string.picker_store_name)) },
        isError = state.nameIssue != null,
        supportingText = state.nameIssue?.let { issue -> { Announced(issue) } },
        singleLine = true,
        enabled = !state.busy,
        keyboardOptions = KeyboardOptions(capitalization = KeyboardCapitalization.Words, imeAction = ImeAction.Next),
        keyboardActions = KeyboardActions(onNext = { focus.moveFocus(FocusDirection.Down) }),
        modifier = Modifier.fillMaxWidth(),
    )
    OutlinedTextField(
        value = state.slug,
        onValueChange = actions.onSlugChange,
        label = { Text(stringResource(Res.string.picker_store_link)) },
        isError = state.slugTaken || state.slugIssue != null,
        supportingText = { LinkHint(state) },
        singleLine = true,
        enabled = !state.busy,
        keyboardOptions =
            KeyboardOptions(keyboardType = KeyboardType.Uri, imeAction = ImeAction.Done, autoCorrectEnabled = false),
        keyboardActions = KeyboardActions(onDone = { actions.onCreate() }),
        modifier = Modifier.fillMaxWidth(),
    )
    BusyButton(
        label = stringResource(Res.string.picker_create),
        busyLabel = stringResource(Res.string.picker_creating),
        busy = state.creating,
        enabled = state.canCreate,
        onClick = actions.onCreate,
        // The main action only without stores; beside a list to choose from, as on the web.
        primary = primary,
    )
}

/** What the link field says: why it was refused, or where the store will live, or the rule. */
@Composable
private fun LinkHint(state: StorePickerUiState) {
    when {
        state.slugTaken -> Announced(stringResource(Res.string.picker_link_taken))
        state.slugIssue != null -> Announced(state.slugIssue)
        looksLikeAStoreLink(state.slug) -> Text(stringResource(Res.string.picker_link_hint, storeLink(state.slug)))
        else -> Text(stringResource(Res.string.picker_link_rule))
    }
}

@Composable
private fun Heading(
    text: String,
    style: TextStyle = MaterialTheme.typography.headlineSmall,
) {
    Text(text = text, style = style, modifier = Modifier.semantics { heading() })
}

/** Text announced when it appears: errors show up after the fields were disabled and lost focus. */
@Composable
private fun Announced(
    text: String,
    color: Color = Color.Unspecified,
) {
    Text(text = text, color = color, modifier = Modifier.semantics { liveRegion = LiveRegionMode.Polite })
}

@Composable
private fun PickerError.message(): String =
    when (this) {
        PickerError.NotAMember -> stringResource(Res.string.picker_error_not_member)
        PickerError.CheckFields -> stringResource(Res.string.picker_error_check_fields)
        PickerError.Offline -> stringResource(Res.string.login_error_offline)
        PickerError.ServerUnavailable -> stringResource(Res.string.login_error_server)
        PickerError.Unexpected -> stringResource(Res.string.picker_error_unexpected)
        is PickerError.RateLimited -> rateLimitMessage(retryAfterSeconds)
    }

/** The picker also opens from the management screens, which follow the workshop's 56 dp targets. */
private val TARGET = 56.dp
