package br.com.encomendo.feature.catalogo

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.selection.toggleable
import androidx.compose.material3.FilterChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import br.com.encomendo.core.domain.catalog.Attribute
import br.com.encomendo.core.ui.TouchTarget
import br.com.encomendo.feature.catalogo.resources.Res
import br.com.encomendo.feature.catalogo.resources.form_backup_add
import br.com.encomendo.feature.catalogo.resources.form_backup_remove
import br.com.encomendo.feature.catalogo.resources.form_cost
import br.com.encomendo.feature.catalogo.resources.form_fields_title
import br.com.encomendo.feature.catalogo.resources.form_production_cost
import br.com.encomendo.feature.catalogo.resources.form_production_days
import br.com.encomendo.feature.catalogo.resources.form_supplier_backup
import br.com.encomendo.feature.catalogo.resources.form_supplier_lead
import br.com.encomendo.feature.catalogo.resources.form_supplier_main
import br.com.encomendo.feature.catalogo.resources.form_supplier_new
import br.com.encomendo.feature.catalogo.resources.form_supplier_section
import org.jetbrains.compose.resources.stringResource

/** "Fornecedor": the main supplier, and the backup one once it is added. */
@Composable
internal fun SupplierSection(
    state: ItemFormUiState,
    actions: ItemFormActions,
) {
    SectionTitle(stringResource(Res.string.form_supplier_section))
    SupplierBlock(SupplierSlot.Main, state.fields.main, state, actions)
    val backup = state.fields.backup
    if (backup == null) {
        SecondaryButton(stringResource(Res.string.form_backup_add), { actions.onEdit(null) { it.withBackupAdded() } })
    } else {
        SupplierBlock(SupplierSlot.Backup, backup, state, actions)
        SecondaryButton(
            stringResource(Res.string.form_backup_remove),
            { actions.onEdit(null) { it.copy(backup = null) } },
        )
    }
}

/** The three fields of one supplier: the choice, "Prazo do fornecedor (dias)" and, for the owner, "Custo". */
@Composable
private fun SupplierBlock(
    slot: SupplierSlot,
    fields: SupplierFields,
    state: ItemFormUiState,
    actions: ItemFormActions,
) {
    val labelRes = if (slot == SupplierSlot.Main) Res.string.form_supplier_main else Res.string.form_supplier_backup
    val label = stringResource(labelRes)
    val newSupplier = stringResource(Res.string.form_supplier_new)
    ChoiceField(
        label = label,
        selectedLabel = state.supplierName(fields.supplierId),
        choices = state.suppliers.map { Choice<String?>(it.id, it.name) } + Choice(null, newSupplier),
        // The choice "Novo fornecedor" opens its form instead of picking.
        onPick = { id ->
            if (id == null) {
                actions.onOpenNewSupplier(slot)
            } else {
                actions.onEdit(ItemField.Supplier(slot)) { it.withSupplier(slot, id) }
            }
        },
        problem = state.issue(ItemField.Supplier(slot))?.text(),
    )
    CatalogTextField(
        label = stringResource(Res.string.form_supplier_lead),
        value = fields.leadDays,
        onValueChange = { text ->
            actions.onEdit(ItemField.LeadDays(slot)) { f -> f.withSlot(slot) { it.copy(leadDays = text) } }
        },
        problem = state.issue(ItemField.LeadDays(slot))?.text(),
        keyboardType = KeyboardType.Number,
    )
    if (state.showsCosts) {
        CatalogTextField(
            label = stringResource(Res.string.form_cost),
            value = fields.cost,
            onValueChange = { text ->
                actions.onEdit(
                    ItemField.Cost(slot),
                ) { f -> f.withSlot(slot) { it.copy(cost = text, costTouched = true) } }
            },
            problem = state.issue(ItemField.Cost(slot))?.text(),
            keyboardType = KeyboardType.Decimal,
        )
    }
}

private fun ItemFormActions.onOpenNewSupplier(slot: SupplierSlot) = supplier.onOpen(slot)

/** "Produção própria": "Dias de produção" and, for the owner, "Custo de produção". */
@Composable
internal fun ProductionSection(
    state: ItemFormUiState,
    actions: ItemFormActions,
) {
    CatalogTextField(
        label = stringResource(Res.string.form_production_days),
        value = state.fields.productionDays,
        onValueChange = { text -> actions.onEdit(ItemField.ProductionDays) { it.copy(productionDays = text) } },
        problem = state.issue(ItemField.ProductionDays)?.text(),
        keyboardType = KeyboardType.Number,
    )
    if (state.showsCosts) {
        CatalogTextField(
            label = stringResource(Res.string.form_production_cost),
            value = state.fields.productionCost,
            onValueChange = { text ->
                actions.onEdit(
                    ItemField.ProductionCost,
                ) { it.copy(productionCost = text, productionCostTouched = true) }
            },
            problem = state.issue(ItemField.ProductionCost)?.text(),
            keyboardType = KeyboardType.Decimal,
        )
    }
}

/** "Campos": one multi-select per store field, as chips of its values; [onPick] tells a value ticked or not. */
@Composable
internal fun AttributePicks(
    attributes: List<Attribute>,
    picked: Set<String>,
    onPick: (valueId: String, picked: Boolean) -> Unit,
) {
    if (attributes.isEmpty()) return
    SectionTitle(stringResource(Res.string.form_fields_title))
    attributes.forEach { attribute ->
        Text(text = attribute.name, style = MaterialTheme.typography.labelLarge)
        FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            attribute.values.forEach { value ->
                val isPicked = value.id in picked
                FilterChip(
                    selected = isPicked,
                    onClick = { onPick(value.id, !isPicked) },
                    label = { Text(value.label, style = MaterialTheme.typography.bodyLarge) },
                    modifier = Modifier.heightIn(min = TouchTarget),
                )
            }
        }
    }
}

/** A labelled switch whose whole row is the touch target. */
@Composable
internal fun SwitchRow(
    label: String,
    checked: Boolean,
    enabled: Boolean,
    onChange: (Boolean) -> Unit,
) {
    Row(
        modifier =
            Modifier
                .fillMaxWidth()
                .heightIn(min = TouchTarget)
                .toggleable(value = checked, enabled = enabled, role = Role.Switch, onValueChange = onChange),
        horizontalArrangement = Arrangement.SpaceBetween,
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(text = label, style = MaterialTheme.typography.bodyLarge, modifier = Modifier.weight(1f))
        Switch(checked = checked, onCheckedChange = null, enabled = enabled)
    }
}
