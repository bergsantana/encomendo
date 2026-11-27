package br.com.encomendo.feature.catalogo

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.unit.dp
import br.com.encomendo.core.domain.catalog.CustomizationOption
import br.com.encomendo.core.domain.catalog.SupplierItem
import br.com.encomendo.feature.catalogo.resources.Res
import br.com.encomendo.feature.catalogo.resources.col_lead
import br.com.encomendo.feature.catalogo.resources.col_price
import br.com.encomendo.feature.catalogo.resources.form_category
import br.com.encomendo.feature.catalogo.resources.form_description
import br.com.encomendo.feature.catalogo.resources.form_name
import br.com.encomendo.feature.catalogo.resources.form_production_days
import br.com.encomendo.feature.catalogo.resources.form_published
import br.com.encomendo.feature.catalogo.resources.form_source
import br.com.encomendo.feature.catalogo.resources.form_supplier_backup
import br.com.encomendo.feature.catalogo.resources.form_supplier_main
import br.com.encomendo.feature.catalogo.resources.items_archived
import br.com.encomendo.feature.catalogo.resources.lead_days
import br.com.encomendo.feature.catalogo.resources.no_category
import br.com.encomendo.feature.catalogo.resources.option_required
import br.com.encomendo.feature.catalogo.resources.options_title
import org.jetbrains.compose.resources.pluralStringResource
import org.jetbrains.compose.resources.stringResource

/**
 * ItemReadOnly shows an item to a role that cannot change the catalog: the same facts as the form, with no field
 * and no button that changes anything. Costs never show here: a role that reads only is never sent one.
 */
@Composable
internal fun ItemReadOnly(state: ItemFormUiState) {
    val item = state.item ?: return
    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        if (item.archived) {
            Text(
                stringResource(Res.string.items_archived),
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        ItemLine(stringResource(Res.string.form_name), item.name)
        if (item.description.isNotBlank()) ItemLine(stringResource(Res.string.form_description), item.description)
        ItemLine(
            stringResource(Res.string.form_category),
            state.categories.firstOrNull { it.id == item.categoryId }?.name ?: stringResource(Res.string.no_category),
        )
        ItemLine(stringResource(Res.string.form_source), item.sourcingMode.label())
        ItemLine(stringResource(Res.string.col_price), formatMoney(item.price))
        item.productionDays?.let { ItemLine(stringResource(Res.string.form_production_days), it.toString()) }
        item.mainSupplier?.let { SupplierLine(stringResource(Res.string.form_supplier_main), it) }
        item.backupSupplier?.let { SupplierLine(stringResource(Res.string.form_supplier_backup), it) }
        AttributeLines(state)
        item.options.forEach { OptionLines(it) }
        // Shown as the switch it is in the form, but nothing can flip it here.
        SwitchRow(
            label = stringResource(Res.string.form_published),
            checked = item.published,
            enabled = false,
            onChange = {},
        )
    }
}

@Composable
private fun SupplierLine(
    label: String,
    supplier: SupplierItem,
) {
    val days = pluralStringResource(Res.plurals.lead_days, supplier.leadDays, supplier.leadDays)
    ItemLine(label, supplier.supplierName)
    ItemLine(stringResource(Res.string.col_lead), days)
}

@Composable
private fun AttributeLines(state: ItemFormUiState) {
    val picked =
        state.item
            ?.attributeValueIds
            .orEmpty()
            .toSet()
    state.attributes.forEach { attribute ->
        val labels = attribute.values.filter { it.id in picked }.joinToString(", ") { it.label }
        if (labels.isNotEmpty()) ItemLine(attribute.name, labels)
    }
}

@Composable
private fun OptionLines(option: CustomizationOption) {
    val required = if (option.required) " (${stringResource(Res.string.option_required)})" else ""
    ItemLine(stringResource(Res.string.options_title), option.name + required)
    Text(text = option.choices.joinToString(", ") { it.label }, style = MaterialTheme.typography.bodyLarge)
}
