package br.com.encomendo.feature.catalogo

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.PaddingValues
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
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import br.com.encomendo.feature.catalogo.resources.Res
import br.com.encomendo.feature.catalogo.resources.attribute_ask_remove
import br.com.encomendo.feature.catalogo.resources.attribute_ask_remove_field
import br.com.encomendo.feature.catalogo.resources.attribute_cancel
import br.com.encomendo.feature.catalogo.resources.attribute_confirm
import br.com.encomendo.feature.catalogo.resources.attribute_delete
import br.com.encomendo.feature.catalogo.resources.attribute_name
import br.com.encomendo.feature.catalogo.resources.attribute_new
import br.com.encomendo.feature.catalogo.resources.attribute_save
import br.com.encomendo.feature.catalogo.resources.attribute_value_add
import br.com.encomendo.feature.catalogo.resources.attribute_value_n
import br.com.encomendo.feature.catalogo.resources.attribute_value_remove
import br.com.encomendo.feature.catalogo.resources.attribute_value_remove_n
import br.com.encomendo.feature.catalogo.resources.attributes_hint
import br.com.encomendo.feature.catalogo.resources.attributes_loading
import br.com.encomendo.feature.catalogo.resources.attributes_title
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
                onNewField = viewModel::onNewField,
                onNameChange = viewModel::onNameChange,
                onValueChange = viewModel::onValueChange,
                onAddValue = viewModel::onAddValue,
                onRemoveValue = viewModel::onRemoveValue,
                onSaveField = viewModel::onSaveField,
                onDeleteField = viewModel::onDeleteField,
                onCancelQuestion = viewModel::onCancelQuestion,
                onConfirmQuestion = viewModel::onConfirmQuestion,
                onRetry = viewModel::onRetry,
            )
        }
    CatalogoTheme { AttributesScreen(state = state, actions = actions, modifier = modifier) }
}

/** AttributesActions are what the user can do on [AttributesScreen]. */
data class AttributesActions(
    val onNewField: () -> Unit,
    val onNameChange: (AttributeEditor, String) -> Unit,
    val onValueChange: (AttributeEditor, ValueEditor, String) -> Unit,
    val onAddValue: (AttributeEditor) -> Unit,
    val onRemoveValue: (AttributeEditor, ValueEditor) -> Unit,
    val onSaveField: (AttributeEditor) -> Unit,
    val onDeleteField: (AttributeEditor) -> Unit,
    val onCancelQuestion: () -> Unit,
    val onConfirmQuestion: () -> Unit,
    val onRetry: () -> Unit,
)

/**
 * AttributesScreen is "Campos dos itens" (docs/product/design-document.md, "Campos"): each field with its values as
 * a draft that "Salvar campo" saves, "Novo campo", and the question asked before removing a value or field that items
 * and kits use. A role that cannot write sees the fields only.
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
        items(state.editors, key = { it.key }) { editor -> AttributeCard(editor, state, actions) }
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
        if (state.loading && state.editors.isEmpty()) Text(stringResource(Res.string.attributes_loading))
        if (state.message != null && state.editors.isEmpty() && !state.loading) {
            SecondaryButton(stringResource(Res.string.items_retry), actions.onRetry)
        }
        if (state.canWrite) {
            PrimaryButton(stringResource(Res.string.attribute_new), actions.onNewField, enabled = !state.busy)
        }
    }
}

@Composable
private fun AttributeCard(
    editor: AttributeEditor,
    state: AttributesUiState,
    actions: AttributesActions,
) {
    OutlinedCard(modifier = Modifier.fillMaxWidth()) {
        Column(modifier = Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            if (state.canWrite) {
                CatalogTextField(
                    label = stringResource(Res.string.attribute_name),
                    value = editor.name,
                    onValueChange = { actions.onNameChange(editor, it) },
                    problem = editor.nameProblem?.attributeText(),
                )
            } else {
                Text(text = editor.name, style = MaterialTheme.typography.titleMedium)
            }
            editor.values.forEachIndexed { index, value -> ValueRow(editor, value, index + 1, state, actions) }
            if (state.canWrite) FieldButtons(editor, state, actions)
        }
    }
}

@Composable
private fun ValueRow(
    editor: AttributeEditor,
    value: ValueEditor,
    number: Int,
    state: AttributesUiState,
    actions: AttributesActions,
) {
    if (!state.canWrite) {
        Text(text = value.label, style = MaterialTheme.typography.bodyLarge)
        return
    }
    val remove = stringResource(Res.string.attribute_value_remove_n, number)
    CatalogTextField(
        label = stringResource(Res.string.attribute_value_n, number),
        value = value.label,
        onValueChange = { actions.onValueChange(editor, value, it) },
        problem = value.problem?.text(),
    )
    SecondaryButton(
        text = stringResource(Res.string.attribute_value_remove),
        onClick = { actions.onRemoveValue(editor, value) },
        modifier = Modifier.semantics { contentDescription = remove },
        enabled = !state.busy,
    )
}

@Composable
private fun FieldButtons(
    editor: AttributeEditor,
    state: AttributesUiState,
    actions: AttributesActions,
) {
    FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
        SecondaryButton(
            text = stringResource(Res.string.attribute_value_add),
            onClick = { actions.onAddValue(editor) },
            enabled = !state.busy,
        )
        SecondaryButton(
            text = stringResource(Res.string.attribute_save),
            onClick = { actions.onSaveField(editor) },
            enabled = !state.busy,
        )
        SecondaryButton(
            text = stringResource(Res.string.attribute_delete),
            onClick = { actions.onDeleteField(editor) },
            enabled = !state.busy,
        )
    }
}

/** The question before a removal; "Cancelar" has the focus's first place and "Remover" is not the primary button. */
@Composable
private fun RemovalDialog(
    question: RemovalQuestion,
    actions: AttributesActions,
) {
    val text =
        when (question) {
            is RemovalQuestion.OfValue -> stringResource(Res.string.attribute_ask_remove, question.name, question.count)
            is RemovalQuestion.OfField -> stringResource(Res.string.attribute_ask_remove_field, question.name)
        }
    AlertDialog(
        onDismissRequest = actions.onCancelQuestion,
        text = { Text(text = text, style = MaterialTheme.typography.bodyLarge) },
        confirmButton = { SecondaryButton(stringResource(Res.string.attribute_confirm), actions.onConfirmQuestion) },
        dismissButton = { PrimaryButton(stringResource(Res.string.attribute_cancel), actions.onCancelQuestion) },
    )
}
