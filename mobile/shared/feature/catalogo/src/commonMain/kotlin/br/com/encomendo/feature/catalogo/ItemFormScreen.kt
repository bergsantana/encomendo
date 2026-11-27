package br.com.encomendo.feature.catalogo

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import br.com.encomendo.core.domain.catalog.Photo
import br.com.encomendo.core.domain.catalog.SourcingMode
import br.com.encomendo.feature.catalogo.resources.Res
import br.com.encomendo.feature.catalogo.resources.form_archive
import br.com.encomendo.feature.catalogo.resources.form_cancel
import br.com.encomendo.feature.catalogo.resources.form_category
import br.com.encomendo.feature.catalogo.resources.form_description
import br.com.encomendo.feature.catalogo.resources.form_edit
import br.com.encomendo.feature.catalogo.resources.form_loading
import br.com.encomendo.feature.catalogo.resources.form_name
import br.com.encomendo.feature.catalogo.resources.form_new
import br.com.encomendo.feature.catalogo.resources.form_price
import br.com.encomendo.feature.catalogo.resources.form_published
import br.com.encomendo.feature.catalogo.resources.form_restore
import br.com.encomendo.feature.catalogo.resources.form_save
import br.com.encomendo.feature.catalogo.resources.form_source
import br.com.encomendo.feature.catalogo.resources.items_retry
import br.com.encomendo.feature.catalogo.resources.no_category
import org.jetbrains.compose.resources.stringResource
import org.koin.compose.viewmodel.koinViewModel
import org.koin.core.parameter.parametersOf

/** ItemFormRoute connects [ItemFormViewModel] for [itemId] (null for a new item) to [ItemFormScreen]. */
@Composable
fun ItemFormRoute(
    itemId: String?,
    onClose: () -> Unit,
    modifier: Modifier = Modifier,
    viewModel: ItemFormViewModel = koinViewModel(parameters = { parametersOf(itemId) }),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    // The effect only asks the screen to leave; nothing here calls the API (ADR 0018).
    LaunchedEffect(viewModel) {
        viewModel.effects.collect { effect ->
            when (effect) {
                ItemFormEffect.Close -> onClose()
            }
        }
    }
    val actions =
        remember(viewModel, onClose) {
            ItemFormActions(
                onEdit = viewModel::onEdit,
                onModeChange = viewModel::onModeChange,
                onSave = viewModel::onSave,
                onCancel = onClose,
                onArchive = viewModel::onArchive,
                onRestore = viewModel::onRestore,
                onReload = viewModel::onReload,
                onAddPhoto = viewModel::onAddPhoto,
                onMakeCover = viewModel::onMakeCover,
                onRemovePhoto = viewModel::onRemovePhoto,
                supplier =
                    NewSupplierActions(
                        onOpen = viewModel::onOpenNewSupplier,
                        onNameChange = viewModel::onNewSupplierName,
                        onPhoneChange = viewModel::onNewSupplierPhone,
                        onAdd = viewModel::onAddSupplier,
                        onClose = viewModel::onCloseNewSupplier,
                    ),
            )
        }
    CatalogoTheme { ItemFormScreen(state = state, actions = actions, modifier = modifier) }
}

/** ItemFormActions are what the user can do on [ItemFormScreen]. */
data class ItemFormActions(
    val onEdit: (ItemField?, (ItemFields) -> ItemFields) -> Unit,
    val onModeChange: (SourcingMode) -> Unit,
    val onSave: () -> Unit,
    val onCancel: () -> Unit,
    val onArchive: () -> Unit,
    val onRestore: () -> Unit,
    val onReload: () -> Unit,
    val onAddPhoto: (PhotoSource) -> Unit,
    val onMakeCover: (Photo) -> Unit,
    val onRemovePhoto: (Photo) -> Unit,
    val supplier: NewSupplierActions,
)

/** NewSupplierActions are what the user can do in "Novo fornecedor". */
data class NewSupplierActions(
    val onOpen: (SupplierSlot) -> Unit,
    val onNameChange: (String) -> Unit,
    val onPhoneChange: (String) -> Unit,
    val onAdd: () -> Unit,
    val onClose: () -> Unit,
)

/**
 * ItemFormScreen is the item editor of Catálogo, as a full screen (docs/product/design-document.md, "Item form").
 * A role without `catalog:write` gets the item read-only, with no field to edit and no button that changes
 * anything. Cost fields exist only when [ItemFormUiState.showsCosts].
 */
@Composable
fun ItemFormScreen(
    state: ItemFormUiState,
    actions: ItemFormActions,
    modifier: Modifier = Modifier,
) {
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
            text = stringResource(if (state.isNew) Res.string.form_new else Res.string.form_edit),
            style = MaterialTheme.typography.headlineSmall,
            modifier = Modifier.semantics { heading() },
        )
        state.offlineSince?.let { OfflineBanner(savedAtEpochMillis = it) }
        when {
            state.loading -> Text(stringResource(Res.string.form_loading), style = MaterialTheme.typography.bodyLarge)
            state.loadMessage != null -> LoadFailed(state.loadMessage, actions.onReload)
            state.canWrite -> EditableForm(state, actions)
            else -> ItemReadOnly(state)
        }
    }
    state.newSupplier?.let { NewSupplierDialog(form = it, actions = actions.supplier) }
}

@Composable
private fun LoadFailed(
    message: CatalogMessage,
    onReload: () -> Unit,
) {
    MessageText(message = message, onReload = onReload)
    SecondaryButton(stringResource(Res.string.items_retry), onReload)
}

@Composable
private fun EditableForm(
    state: ItemFormUiState,
    actions: ItemFormActions,
) {
    state.message?.let { MessageText(message = it, onReload = actions.onReload) }
    BasicFields(state, actions)
    if (state.fields.mode == SourcingMode.InHouse) {
        ProductionSection(state, actions)
    } else {
        SupplierSection(state, actions)
    }
    AttributePicks(state, actions)
    OptionsEditor(state, actions)
    PhotosEditor(state, actions)
    SwitchRow(
        label = stringResource(Res.string.form_published),
        checked = state.fields.published,
        enabled = !state.archived,
        onChange = { checked -> actions.onEdit(null) { it.copy(published = checked) } },
    )
    FormButtons(state, actions)
}

@Composable
private fun BasicFields(
    state: ItemFormUiState,
    actions: ItemFormActions,
) {
    val fields = state.fields
    CatalogTextField(
        label = stringResource(Res.string.form_name),
        value = fields.name,
        onValueChange = { text -> actions.onEdit(ItemField.Name) { it.copy(name = text) } },
        problem = state.issue(ItemField.Name)?.text(),
    )
    CatalogTextField(
        label = stringResource(Res.string.form_description),
        value = fields.description,
        onValueChange = { text -> actions.onEdit(null) { it.copy(description = text) } },
        singleLine = false,
    )
    val noCategory = stringResource(Res.string.no_category)
    ChoiceField(
        label = stringResource(Res.string.form_category),
        selectedLabel = state.categories.firstOrNull { it.id == fields.categoryId }?.name ?: noCategory,
        choices = listOf(Choice<String?>(null, noCategory)) + state.categories.map { Choice(it.id, it.name) },
        onPick = { id -> actions.onEdit(null) { it.copy(categoryId = id) } },
    )
    ChoiceField(
        label = stringResource(Res.string.form_source),
        selectedLabel = fields.mode.label(),
        choices = state.modes.map { Choice(it, it.label()) },
        onPick = actions.onModeChange,
    )
    CatalogTextField(
        label = stringResource(Res.string.form_price),
        value = fields.price,
        onValueChange = { text -> actions.onEdit(ItemField.Price) { it.copy(price = text) } },
        problem = state.issue(ItemField.Price)?.text(),
        keyboardType = KeyboardType.Decimal,
    )
}

@Composable
private fun FormButtons(
    state: ItemFormUiState,
    actions: ItemFormActions,
) {
    PrimaryButton(
        text = stringResource(Res.string.form_save),
        onClick = actions.onSave,
        enabled = !state.saving,
    )
    SecondaryButton(stringResource(Res.string.form_cancel), actions.onCancel, enabled = !state.saving)
    if (!state.isNew) {
        if (state.archived) {
            SecondaryButton(stringResource(Res.string.form_restore), actions.onRestore, enabled = !state.saving)
        } else {
            SecondaryButton(stringResource(Res.string.form_archive), actions.onArchive, enabled = !state.saving)
        }
    }
}
