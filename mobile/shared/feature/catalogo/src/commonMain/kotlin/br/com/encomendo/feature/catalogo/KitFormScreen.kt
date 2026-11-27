package br.com.encomendo.feature.catalogo

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.selection.selectableGroup
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.RadioButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.ExperimentalComposeUiApi
import androidx.compose.ui.Modifier
import androidx.compose.ui.backhandler.BackHandler
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import br.com.encomendo.core.domain.catalog.SwapDiscount
import br.com.encomendo.core.ui.TouchTarget
import br.com.encomendo.feature.catalogo.resources.Res
import br.com.encomendo.feature.catalogo.resources.form_archive
import br.com.encomendo.feature.catalogo.resources.form_cancel
import br.com.encomendo.feature.catalogo.resources.form_close
import br.com.encomendo.feature.catalogo.resources.form_description
import br.com.encomendo.feature.catalogo.resources.form_name
import br.com.encomendo.feature.catalogo.resources.form_published
import br.com.encomendo.feature.catalogo.resources.form_restore
import br.com.encomendo.feature.catalogo.resources.form_save
import br.com.encomendo.feature.catalogo.resources.kit_assembly_days
import br.com.encomendo.feature.catalogo.resources.kit_edit
import br.com.encomendo.feature.catalogo.resources.kit_loading
import br.com.encomendo.feature.catalogo.resources.kit_new
import br.com.encomendo.feature.catalogo.resources.kit_price
import br.com.encomendo.feature.catalogo.resources.kit_readonly_title
import br.com.encomendo.feature.catalogo.resources.kit_safety_margin_days
import br.com.encomendo.feature.catalogo.resources.kit_saved
import br.com.encomendo.feature.catalogo.resources.kit_swap_keep
import br.com.encomendo.feature.catalogo.resources.kit_swap_keep_hint
import br.com.encomendo.feature.catalogo.resources.kit_swap_lose
import br.com.encomendo.feature.catalogo.resources.kit_swap_lose_hint
import br.com.encomendo.feature.catalogo.resources.kit_swap_title
import org.jetbrains.compose.resources.StringResource
import org.jetbrains.compose.resources.stringResource
import org.koin.compose.viewmodel.koinViewModel
import org.koin.core.parameter.parametersOf

/** KitFormRoute connects [KitFormViewModel] for [kitId] (null for a new kit) to [KitFormScreen]. */
@Composable
fun KitFormRoute(
    kitId: String?,
    onClose: () -> Unit,
    modifier: Modifier = Modifier,
    viewModel: KitFormViewModel = koinViewModel(parameters = { parametersOf(kitId) }),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    // The effect only asks the screen to leave; nothing here calls the API (ADR 0018).
    LaunchedEffect(viewModel) {
        viewModel.effects.collect { effect ->
            when (effect) {
                KitFormEffect.Close -> onClose()
            }
        }
    }
    val actions =
        remember(viewModel) {
            KitFormActions(
                onEdit = viewModel::onEdit,
                onPickItemToAdd = viewModel::onPickItemToAdd,
                onAddItem = viewModel::onAddItem,
                onRemoveItem = viewModel::onRemoveItem,
                onSave = viewModel::onSave,
                onCancel = viewModel::onCancel,
                onArchive = viewModel::onArchive,
                onRestore = viewModel::onRestore,
                onReload = viewModel::onReload,
                onKeepEditing = viewModel::onKeepEditing,
                onDiscard = viewModel::onDiscard,
                photos =
                    PhotoActions(
                        onAdd = viewModel::onAddPhoto,
                        onMakeCover = viewModel::onMakeCover,
                        onRemove = viewModel::onRemovePhoto,
                    ),
            )
        }
    CatalogoTheme { KitFormScreen(state = state, actions = actions, modifier = modifier) }
}

/** KitFormActions are what the user can do on [KitFormScreen]. */
data class KitFormActions(
    val onEdit: (KitField?, (KitFields) -> KitFields) -> Unit,
    val onPickItemToAdd: (String?) -> Unit,
    val onAddItem: () -> Unit,
    val onRemoveItem: (itemId: String) -> Unit,
    val onSave: () -> Unit,
    val onCancel: () -> Unit,
    val onArchive: () -> Unit,
    val onRestore: () -> Unit,
    val onReload: () -> Unit,
    val onKeepEditing: () -> Unit,
    val onDiscard: () -> Unit,
    val photos: PhotoActions,
)

/**
 * KitFormScreen is the kit editor of Catálogo, as a full screen (docs/product/design-document.md, "Catálogo",
 * "Kits"). A role without `catalog:write` gets the kit read-only, with no field to edit and no button that changes
 * anything. "Margem" shows only when the API sent it. System Back is "Cancelar" through `BackHandler`.
 */
@OptIn(ExperimentalComposeUiApi::class)
@Composable
fun KitFormScreen(
    state: KitFormUiState,
    actions: KitFormActions,
    modifier: Modifier = Modifier,
) {
    BackHandler(enabled = true, onBack = actions.onCancel)
    Column(
        modifier =
            modifier
                .fillMaxSize()
                .imePadding()
                .verticalScroll(rememberScrollState())
                .padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        Text(
            text = stringResource(titleOf(state)),
            style = MaterialTheme.typography.headlineSmall,
            modifier = Modifier.semantics { heading() },
        )
        state.offlineSince?.let { OfflineBanner(savedAtEpochMillis = it) }
        when {
            state.loading ->
                Text(
                    stringResource(Res.string.kit_loading),
                    style = MaterialTheme.typography.bodyLarge,
                )
            state.loadMessage != null -> LoadFailed(state.loadMessage, actions.onReload)
            state.canWrite -> EditableKit(state, actions)
            else -> {
                KitReadOnly(state)
                SecondaryButton(stringResource(Res.string.form_close), actions.onCancel)
            }
        }
    }
    state.discard?.let { DiscardDialog(onKeepEditing = actions.onKeepEditing, onDiscard = actions.onDiscard) }
}

/** "Kit" for a role that only reads it, else "Novo kit" or "Editar kit". */
private fun titleOf(state: KitFormUiState): StringResource =
    when {
        !state.loading && !state.canWrite && state.loadMessage == null -> Res.string.kit_readonly_title
        state.isNew -> Res.string.kit_new
        else -> Res.string.kit_edit
    }

@Composable
private fun EditableKit(
    state: KitFormUiState,
    actions: KitFormActions,
) {
    if (state.saved) {
        Text(
            stringResource(Res.string.kit_saved),
            style = MaterialTheme.typography.bodyLarge,
            modifier = Modifier.announced(),
        )
    }
    state.message?.let { MessageText(message = it, onReload = actions.onReload) }
    KitBasics(state, actions)
    KitItemsEditor(state, actions)
    SwapModes(state.fields.swapDiscount) { mode -> actions.onEdit(null) { it.copy(swapDiscount = mode) } }
    KitDays(state, actions)
    KitTotals(state)
    AttributePicks(state.attributes, state.fields.attributeValueIds) { id, picked ->
        actions.onEdit(null) { it.withValuePicked(id, picked) }
    }
    PhotosEditor(state.photos, actions.photos)
    SwitchRow(
        label = stringResource(Res.string.form_published),
        checked = state.fields.published,
        enabled = !state.archived,
        onChange = { checked -> actions.onEdit(null) { it.copy(published = checked) } },
    )
    KitButtons(state, actions)
}

@Composable
private fun KitBasics(
    state: KitFormUiState,
    actions: KitFormActions,
) {
    CatalogTextField(
        label = stringResource(Res.string.form_name),
        value = state.fields.name,
        onValueChange = { text -> actions.onEdit(KitField.Name) { it.copy(name = text) } },
        problem = state.issue(KitField.Name)?.text(),
    )
    CatalogTextField(
        label = stringResource(Res.string.form_description),
        value = state.fields.description,
        onValueChange = { text -> actions.onEdit(null) { it.copy(description = text) } },
        singleLine = false,
    )
    CatalogTextField(
        label = stringResource(Res.string.kit_price),
        value = state.fields.price,
        onValueChange = { text -> actions.onEdit(KitField.Price) { it.copy(price = text) } },
        problem = state.issue(KitField.Price)?.text(),
        keyboardType = KeyboardType.Decimal,
    )
}

/** "Ao trocar um item": keep or lose the kit's discount, each with its hint. */
@Composable
private fun SwapModes(
    selected: SwapDiscount,
    onPick: (SwapDiscount) -> Unit,
) {
    SectionTitle(stringResource(Res.string.kit_swap_title))
    Column(modifier = Modifier.selectableGroup(), verticalArrangement = Arrangement.spacedBy(8.dp)) {
        SwapMode(SwapDiscount.Keep, selected, Res.string.kit_swap_keep, Res.string.kit_swap_keep_hint, onPick)
        SwapMode(SwapDiscount.Lose, selected, Res.string.kit_swap_lose, Res.string.kit_swap_lose_hint, onPick)
    }
}

@Composable
private fun SwapMode(
    mode: SwapDiscount,
    selected: SwapDiscount,
    label: StringResource,
    hint: StringResource,
    onPick: (SwapDiscount) -> Unit,
) {
    Column {
        Row(
            modifier =
                Modifier
                    .fillMaxWidth()
                    .heightIn(min = TouchTarget)
                    .selectable(selected = mode == selected, role = Role.RadioButton, onClick = { onPick(mode) }),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            RadioButton(selected = mode == selected, onClick = null)
            Text(
                stringResource(label),
                style = MaterialTheme.typography.bodyLarge,
                modifier = Modifier.padding(start = 12.dp),
            )
        }
        Hint(stringResource(hint))
    }
}

/** "Dias de {Montagem}" and "Margem de segurança (dias)": blank means the store's default, which the hint names. */
@Composable
private fun KitDays(
    state: KitFormUiState,
    actions: KitFormActions,
) {
    CatalogTextField(
        label = stringResource(Res.string.kit_assembly_days, state.niche.montagem),
        value = state.fields.assemblyDays,
        onValueChange = { text -> actions.onEdit(KitField.AssemblyDays) { it.copy(assemblyDays = text) } },
        problem = state.issue(KitField.AssemblyDays)?.text(),
        hint = state.defaultAssemblyDays?.let { defaultDaysHint(it) },
        keyboardType = KeyboardType.Number,
    )
    CatalogTextField(
        label = stringResource(Res.string.kit_safety_margin_days),
        value = state.fields.safetyMarginDays,
        onValueChange = { text -> actions.onEdit(KitField.SafetyMarginDays) { it.copy(safetyMarginDays = text) } },
        problem = state.issue(KitField.SafetyMarginDays)?.text(),
        hint = state.defaultSafetyMarginDays?.let { defaultDaysHint(it) },
        keyboardType = KeyboardType.Number,
    )
}

@Composable
private fun KitButtons(
    state: KitFormUiState,
    actions: KitFormActions,
) {
    PrimaryButton(text = stringResource(Res.string.form_save), onClick = actions.onSave, enabled = !state.saving)
    SecondaryButton(stringResource(Res.string.form_cancel), actions.onCancel, enabled = !state.saving)
    if (!state.isNew) {
        if (state.archived) {
            SecondaryButton(stringResource(Res.string.form_restore), actions.onRestore, enabled = !state.saving)
        } else {
            SecondaryButton(stringResource(Res.string.form_archive), actions.onArchive, enabled = !state.saving)
        }
    }
}
