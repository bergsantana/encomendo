package br.com.encomendo.feature.catalogo

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.unit.dp
import br.com.encomendo.core.domain.catalog.Kit
import br.com.encomendo.core.domain.catalog.KitItem
import br.com.encomendo.core.domain.catalog.SwapDiscount
import br.com.encomendo.feature.catalogo.resources.Res
import br.com.encomendo.feature.catalogo.resources.form_description
import br.com.encomendo.feature.catalogo.resources.form_name
import br.com.encomendo.feature.catalogo.resources.form_published
import br.com.encomendo.feature.catalogo.resources.items_archived
import br.com.encomendo.feature.catalogo.resources.kit_assembly_days
import br.com.encomendo.feature.catalogo.resources.kit_item_quantity
import br.com.encomendo.feature.catalogo.resources.kit_item_swap_for
import br.com.encomendo.feature.catalogo.resources.kit_item_swappable
import br.com.encomendo.feature.catalogo.resources.kit_items_title
import br.com.encomendo.feature.catalogo.resources.kit_price
import br.com.encomendo.feature.catalogo.resources.kit_safety_margin_days
import br.com.encomendo.feature.catalogo.resources.kit_swap_keep
import br.com.encomendo.feature.catalogo.resources.kit_swap_lose
import br.com.encomendo.feature.catalogo.resources.kit_swap_title
import org.jetbrains.compose.resources.stringResource

/**
 * KitReadOnly shows a kit to a role that cannot change the catalog: the same facts as the editor in labelled lines,
 * with no field and no button that changes anything. The totals are the API's, and "Margem" is never there: a role
 * that only reads is never sent one.
 */
@Composable
internal fun KitReadOnly(state: KitFormUiState) {
    val kit = state.kit ?: return
    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        if (kit.archived) {
            Text(stringResource(Res.string.items_archived), color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
        ItemLine(stringResource(Res.string.form_name), kit.name)
        if (kit.description.isNotBlank()) ItemLine(stringResource(Res.string.form_description), kit.description)
        ItemLine(stringResource(Res.string.kit_price), formatMoney(kit.price))
        SectionTitle(stringResource(Res.string.kit_items_title))
        kit.items.forEach { KitItemLines(it, state) }
        ItemLine(stringResource(Res.string.kit_swap_title), kit.swapDiscount.label())
        ItemLine(stringResource(Res.string.kit_assembly_days, state.niche.montagem), kit.days.assembly.toString())
        ItemLine(stringResource(Res.string.kit_safety_margin_days), kit.days.safetyMargin.toString())
        KitTotals(state)
        AttributeLines(state, kit)
        // Shown as the switch it is in the editor, but nothing can flip it here.
        SwitchRow(
            label = stringResource(Res.string.form_published),
            checked = kit.published,
            enabled = false,
            onChange = {},
        )
    }
}

@Composable
private fun SwapDiscount.label(): String =
    stringResource(if (this == SwapDiscount.Keep) Res.string.kit_swap_keep else Res.string.kit_swap_lose)

@Composable
private fun KitItemLines(
    row: KitItem,
    state: KitFormUiState,
) {
    val archived = if (row.archived) " (${stringResource(Res.string.items_archived)})" else ""
    Text(text = row.itemName + archived, style = MaterialTheme.typography.titleMedium)
    ItemLine(stringResource(Res.string.kit_item_quantity), row.quantity.toString())
    if (row.swappable) {
        val names = row.alternativeItemIds.mapNotNull { id -> state.catalogItems.firstOrNull { it.id == id }?.name }
        Text(stringResource(Res.string.kit_item_swappable), style = MaterialTheme.typography.bodyLarge)
        if (names.isNotEmpty()) ItemLine(stringResource(Res.string.kit_item_swap_for), names.joinToString(", "))
    }
}

@Composable
private fun AttributeLines(
    state: KitFormUiState,
    kit: Kit,
) {
    val picked = kit.attributeValueIds.toSet()
    state.attributes.forEach { attribute ->
        val labels = attribute.values.filter { it.id in picked }.joinToString(", ") { it.label }
        if (labels.isNotEmpty()) ItemLine(attribute.name, labels)
    }
}
