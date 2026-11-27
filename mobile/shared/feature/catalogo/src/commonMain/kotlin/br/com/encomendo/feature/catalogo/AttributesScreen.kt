package br.com.encomendo.feature.catalogo

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedCard
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import br.com.encomendo.core.domain.catalog.Attribute
import br.com.encomendo.core.domain.catalog.AttributeValue
import br.com.encomendo.feature.catalogo.resources.Res
import br.com.encomendo.feature.catalogo.resources.attribute_ask_remove
import br.com.encomendo.feature.catalogo.resources.attribute_cancel
import br.com.encomendo.feature.catalogo.resources.attribute_confirm
import br.com.encomendo.feature.catalogo.resources.attribute_delete
import br.com.encomendo.feature.catalogo.resources.attribute_name
import br.com.encomendo.feature.catalogo.resources.attribute_new
import br.com.encomendo.feature.catalogo.resources.attribute_value
import br.com.encomendo.feature.catalogo.resources.attribute_value_add
import br.com.encomendo.feature.catalogo.resources.attribute_value_remove
import br.com.encomendo.feature.catalogo.resources.attribute_value_remove_of
import br.com.encomendo.feature.catalogo.resources.attribute_values
import br.com.encomendo.feature.catalogo.resources.attributes_hint
import br.com.encomendo.feature.catalogo.resources.attributes_loading
import br.com.encomendo.feature.catalogo.resources.attributes_title
import br.com.encomendo.feature.catalogo.resources.category_cancel
import br.com.encomendo.feature.catalogo.resources.category_rename
import br.com.encomendo.feature.catalogo.resources.category_save
import br.com.encomendo.feature.catalogo.resources.items_retry
import org.jetbrains.compose.resources.stringResource
import org.koin.compose.viewmodel.koinViewModel

/** AttributesRoute connects [AttributesViewModel] to [AttributesScreen]. */
@Composable
fun AttributesRoute(
    modifier: Modifier = Modifier,
    viewModel: AttributesViewModel = koinViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val actions =
        remember(viewModel) {
            AttributesActions(
                onNewNameChange = viewModel::onNewNameChange,
                onAddField = viewModel::onAddField,
                onValueTextChange = viewModel::onValueTextChange,
                onAddValue = viewModel::onAddValue,
                onRemoveValue = viewModel::onRemoveValue,
                onDeleteField = viewModel::onDeleteField,
                onCancelQuestion = viewModel::onCancelQuestion,
                onConfirmQuestion = viewModel::onConfirmQuestion,
                onStartRename = viewModel::onStartRename,
                onRenameChange = viewModel::onRenameChange,
                onSaveRename = viewModel::onSaveRename,
                onCancelRename = viewModel::onCancelRename,
                onRetry = viewModel::onRetry,
            )
        }
    CatalogoTheme { AttributesScreen(state = state, actions = actions, modifier = modifier) }
}

/** AttributesActions are what the user can do on [AttributesScreen]. */
data class AttributesActions(
    val onNewNameChange: (String) -> Unit,
    val onAddField: () -> Unit,
    val onValueTextChange: (Attribute, String) -> Unit,
    val onAddValue: (Attribute) -> Unit,
    val onRemoveValue: (Attribute, AttributeValue) -> Unit,
    val onDeleteField: (Attribute) -> Unit,
    val onCancelQuestion: () -> Unit,
    val onConfirmQuestion: () -> Unit,
    val onStartRename: (Attribute) -> Unit,
    val onRenameChange: (String) -> Unit,
    val onSaveRename: () -> Unit,
    val onCancelRename: () -> Unit,
    val onRetry: () -> Unit,
)

/**
 * AttributesScreen is "Campos dos itens" (docs/product/design-document.md, "Campos"): the fields such as "Serve
 * em" with their values, "Novo campo", and the question asked before removing a value or field that items and kits
 * use. A role that cannot write sees the fields only.
 */
@Composable
fun AttributesScreen(
    state: AttributesUiState,
    actions: AttributesActions,
    modifier: Modifier = Modifier,
) {
    LazyColumn(
        modifier = modifier.fillMaxSize().imePadding(),
        contentPadding = PaddingValues(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        item(key = "header") { AttributesHeader(state, actions) }
        items(state.attributes, key = { it.id }) { attribute -> AttributeCard(attribute, state, actions) }
    }
    state.question?.let { RemovalDialog(it, actions) }
}

@Composable
private fun AttributesHeader(
    state: AttributesUiState,
    actions: AttributesActions,
) {
    Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Text(
            text = stringResource(Res.string.attributes_title),
            style = MaterialTheme.typography.headlineSmall,
            modifier = Modifier.semantics { heading() },
        )
        Text(
            text = stringResource(Res.string.attributes_hint),
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        state.offlineSince?.let { OfflineBanner(savedAtEpochMillis = it) }
        state.message?.let { MessageText(message = it) }
        if (state.loading && state.attributes.isEmpty()) Text(stringResource(Res.string.attributes_loading))
        if (state.message != null && state.attributes.isEmpty() && !state.loading) {
            SecondaryButton(stringResource(Res.string.items_retry), actions.onRetry)
        }
        if (state.canWrite) {
            CatalogTextField(
                label = stringResource(Res.string.attribute_name),
                value = state.newName,
                onValueChange = actions.onNewNameChange,
                problem = state.newProblem?.attributeText(),
            )
            PrimaryButton(stringResource(Res.string.attribute_new), actions.onAddField, enabled = !state.busy)
        }
    }
}

@Composable
private fun AttributeCard(
    attribute: Attribute,
    state: AttributesUiState,
    actions: AttributesActions,
) {
    OutlinedCard(modifier = Modifier.fillMaxWidth()) {
        Column(modifier = Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            val rename = state.rename?.takeIf { it.attributeId == attribute.id }
            if (rename != null) {
                AttributeRenameFields(rename, state, actions)
            } else {
                Text(text = attribute.name, style = MaterialTheme.typography.titleMedium)
                if (state.canWrite) AttributeButtons(attribute, state, actions)
            }
            Text(text = stringResource(Res.string.attribute_values), style = MaterialTheme.typography.labelLarge)
            attribute.values.forEach { value -> ValueRow(attribute, value, state, actions) }
            if (state.canWrite) NewValue(attribute, state, actions)
        }
    }
}

@Composable
private fun AttributeRenameFields(
    rename: AttributeRename,
    state: AttributesUiState,
    actions: AttributesActions,
) {
    CatalogTextField(
        label = stringResource(Res.string.attribute_name),
        value = rename.name,
        onValueChange = actions.onRenameChange,
        problem = rename.problem?.attributeText(),
    )
    FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
        SecondaryButton(stringResource(Res.string.category_save), actions.onSaveRename, enabled = !state.busy)
        SecondaryButton(stringResource(Res.string.category_cancel), actions.onCancelRename)
    }
}

@Composable
private fun AttributeButtons(
    attribute: Attribute,
    state: AttributesUiState,
    actions: AttributesActions,
) {
    val rename = stringResource(Res.string.category_rename)
    val delete = stringResource(Res.string.attribute_delete)
    FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
        SecondaryButton(
            text = rename,
            onClick = { actions.onStartRename(attribute) },
            modifier = Modifier.semantics { contentDescription = "$rename: ${attribute.name}" },
            enabled = !state.busy,
        )
        SecondaryButton(
            text = delete,
            onClick = { actions.onDeleteField(attribute) },
            modifier = Modifier.semantics { contentDescription = "$delete: ${attribute.name}" },
            enabled = !state.busy,
        )
    }
}

@Composable
private fun ValueRow(
    attribute: Attribute,
    value: AttributeValue,
    state: AttributesUiState,
    actions: AttributesActions,
) {
    Row(
        modifier = Modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.SpaceBetween,
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(text = value.label, style = MaterialTheme.typography.bodyLarge, modifier = Modifier.weight(1f))
        if (state.canWrite) {
            val description = stringResource(Res.string.attribute_value_remove_of, value.label)
            SecondaryButton(
                text = stringResource(Res.string.attribute_value_remove),
                onClick = { actions.onRemoveValue(attribute, value) },
                modifier = Modifier.semantics { contentDescription = description },
                enabled = !state.busy,
            )
        }
    }
}

@Composable
private fun NewValue(
    attribute: Attribute,
    state: AttributesUiState,
    actions: AttributesActions,
) {
    CatalogTextField(
        label = stringResource(Res.string.attribute_value),
        value = state.valueTexts[attribute.id].orEmpty(),
        onValueChange = { actions.onValueTextChange(attribute, it) },
        problem = state.valueProblems[attribute.id]?.text(),
    )
    SecondaryButton(
        text = stringResource(Res.string.attribute_value_add),
        onClick = { actions.onAddValue(attribute) },
        enabled = !state.busy,
    )
}

/** "Remover {valor}? Ele sai de {n} itens e kits." with "Cancelar" and "Remover". */
@Composable
private fun RemovalDialog(
    question: RemovalQuestion,
    actions: AttributesActions,
) {
    AlertDialog(
        onDismissRequest = actions.onCancelQuestion,
        text = {
            Text(
                text = stringResource(Res.string.attribute_ask_remove, question.name, question.count),
                style = MaterialTheme.typography.bodyLarge,
            )
        },
        confirmButton = { PrimaryButton(stringResource(Res.string.attribute_confirm), actions.onConfirmQuestion) },
        dismissButton = { SecondaryButton(stringResource(Res.string.attribute_cancel), actions.onCancelQuestion) },
    )
}
