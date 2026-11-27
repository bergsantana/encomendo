package br.com.encomendo.feature.catalogo

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.selection.toggleable
import androidx.compose.material3.Checkbox
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedCard
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import br.com.encomendo.core.domain.catalog.ItemSummary
import br.com.encomendo.core.ui.TouchTarget
import br.com.encomendo.feature.catalogo.resources.Res
import br.com.encomendo.feature.catalogo.resources.items_archived
import br.com.encomendo.feature.catalogo.resources.kit_item_add
import br.com.encomendo.feature.catalogo.resources.kit_item_archived_alternative
import br.com.encomendo.feature.catalogo.resources.kit_item_none_to_add
import br.com.encomendo.feature.catalogo.resources.kit_item_others
import br.com.encomendo.feature.catalogo.resources.kit_item_pick
import br.com.encomendo.feature.catalogo.resources.kit_item_picker
import br.com.encomendo.feature.catalogo.resources.kit_item_quantity
import br.com.encomendo.feature.catalogo.resources.kit_item_quantity_of
import br.com.encomendo.feature.catalogo.resources.kit_item_remove
import br.com.encomendo.feature.catalogo.resources.kit_item_remove_of
import br.com.encomendo.feature.catalogo.resources.kit_item_suggestions
import br.com.encomendo.feature.catalogo.resources.kit_item_swap_for
import br.com.encomendo.feature.catalogo.resources.kit_item_swappable
import br.com.encomendo.feature.catalogo.resources.kit_item_swappable_of
import br.com.encomendo.feature.catalogo.resources.kit_items_title
import org.jetbrains.compose.resources.stringResource

/**
 * "Itens do kit": the items a kit holds, each with its "Quantidade" and "Cliente pode trocar". A swappable item offers
 * "Pode trocar por", with the items of its own category suggested first. Items are added from a picker of the store's
 * non-archived items; an item archived after the kit was saved stays in its row, marked, until it is taken out.
 */
@Composable
internal fun KitItemsEditor(
    state: KitFormUiState,
    actions: KitFormActions,
) {
    SectionTitle(stringResource(Res.string.kit_items_title))
    state.issue(KitField.Items)?.let { ProblemText(it.text()) }
    state.fields.items.forEach { row -> KitItemRow(row, state, actions) }
    ItemPicker(state, actions)
}

@Composable
internal fun ProblemText(text: String) {
    Text(
        text = text,
        color = MaterialTheme.colorScheme.error,
        style = MaterialTheme.typography.bodyMedium,
        modifier = Modifier.announced(),
    )
}

@Composable
private fun KitItemRow(
    row: KitItemFields,
    state: KitFormUiState,
    actions: KitFormActions,
) {
    val remove = stringResource(Res.string.kit_item_remove)
    val removeOf = stringResource(Res.string.kit_item_remove_of, row.itemName)
    // Each row is read with its item's name: "Quantidade" alone would not say of which.
    val quantityOf = stringResource(Res.string.kit_item_quantity_of, row.itemName)
    val swappableOf = stringResource(Res.string.kit_item_swappable_of, row.itemName)
    OutlinedCard(modifier = Modifier.fillMaxWidth()) {
        Column(modifier = Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.CenterVertically) {
                Text(row.itemName, style = MaterialTheme.typography.titleMedium, modifier = Modifier.weight(1f, false))
                // An item archived after the kit was saved: the API refuses the kit until the row is taken out.
                if (row.archived) {
                    Text(
                        text = stringResource(Res.string.items_archived),
                        style = MaterialTheme.typography.labelLarge,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
            CatalogTextField(
                label = stringResource(Res.string.kit_item_quantity),
                value = row.quantity,
                onValueChange = { text ->
                    actions.onEdit(
                        KitField.Quantity(row.itemId),
                    ) { f -> f.withRow(row.itemId) { it.copy(quantity = text) } }
                },
                problem = state.issue(KitField.Quantity(row.itemId))?.text(),
                keyboardType = KeyboardType.Number,
                modifier = Modifier.semantics { contentDescription = quantityOf },
            )
            CheckRow(
                label = stringResource(Res.string.kit_item_swappable),
                description = swappableOf,
                checked = row.swappable,
                onChange = { actions.onEdit(null) { f -> f.withSwappable(row.itemId, it) } },
            )
            if (row.swappable) Alternatives(row, state, actions)
            SecondaryButton(
                text = remove,
                onClick = { actions.onRemoveItem(row.itemId) },
                modifier = Modifier.semantics { contentDescription = removeOf },
            )
        }
    }
}

/** "Pode trocar por": the suggestions of the row's category first, then the other items. */
@Composable
private fun Alternatives(
    row: KitItemFields,
    state: KitFormUiState,
    actions: KitFormActions,
) {
    Text(stringResource(Res.string.kit_item_swap_for), style = MaterialTheme.typography.titleSmall)
    state.issue(KitField.Alternatives(row.itemId))?.let { ProblemText(it.text()) }
    alternativesFor(row.itemId, row.alternativeItemIds, state.catalogItems, state.categories).forEach { group ->
        group.heading?.let { Text(it.text(), style = MaterialTheme.typography.labelLarge) }
        group.items.forEach { item ->
            CheckRow(
                label = alternativeLabel(item),
                checked = item.id in row.alternativeItemIds,
                onChange = { picked ->
                    actions.onEdit(
                        KitField.Alternatives(row.itemId),
                    ) { it.withAlternative(row.itemId, item.id, picked) }
                },
            )
        }
    }
}

@Composable
private fun AlternativeHeading.text(): String =
    when (this) {
        is AlternativeHeading.Suggestions -> stringResource(Res.string.kit_item_suggestions, categoryName)
        AlternativeHeading.Others -> stringResource(Res.string.kit_item_others)
    }

/** The item's name, with "(arquivado)" while the row still names an archived item so it can be unticked. */
@Composable
internal fun alternativeLabel(item: ItemSummary): String =
    if (item.archived) stringResource(Res.string.kit_item_archived_alternative, item.name) else item.name

/** "Item a adicionar" with "Adicionar item", or "Nenhum item disponível para adicionar.". */
@Composable
private fun ItemPicker(
    state: KitFormUiState,
    actions: KitFormActions,
) {
    val pick = stringResource(Res.string.kit_item_pick)
    val addable = state.addable
    ChoiceField(
        label = stringResource(Res.string.kit_item_picker),
        selectedLabel = addable.firstOrNull { it.id == state.itemToAdd }?.name ?: pick,
        choices = listOf(Choice<String?>(null, pick)) + addable.map { Choice(it.id, it.name) },
        onPick = actions.onPickItemToAdd,
    )
    SecondaryButton(
        text = stringResource(Res.string.kit_item_add),
        onClick = actions.onAddItem,
        enabled = state.itemToAdd != null,
    )
    if (addable.isEmpty()) Hint(stringResource(Res.string.kit_item_none_to_add))
}

/** A labelled box whose whole row is the touch target. */
@Composable
internal fun CheckRow(
    label: String,
    checked: Boolean,
    onChange: (Boolean) -> Unit,
    description: String? = null,
) {
    Row(
        modifier =
            Modifier
                .fillMaxWidth()
                .heightIn(min = TouchTarget)
                .toggleable(value = checked, role = Role.Checkbox, onValueChange = onChange)
                .semantics { description?.let { contentDescription = it } },
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Checkbox(checked = checked, onCheckedChange = null)
        Text(text = label, style = MaterialTheme.typography.bodyLarge, modifier = Modifier.padding(start = 12.dp))
    }
}
