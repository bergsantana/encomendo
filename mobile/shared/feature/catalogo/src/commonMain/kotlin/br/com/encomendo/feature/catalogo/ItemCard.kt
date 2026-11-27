package br.com.encomendo.feature.catalogo

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.selection.toggleable
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedCard
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import br.com.encomendo.core.domain.catalog.ItemSummary
import br.com.encomendo.core.domain.catalog.OwnerField
import br.com.encomendo.core.domain.catalog.valueOrNull
import br.com.encomendo.core.domain.money.Money
import br.com.encomendo.core.ui.PhotoImage
import br.com.encomendo.core.ui.TouchTarget
import br.com.encomendo.feature.catalogo.resources.Res
import br.com.encomendo.feature.catalogo.resources.col_category
import br.com.encomendo.feature.catalogo.resources.col_cost
import br.com.encomendo.feature.catalogo.resources.col_lead
import br.com.encomendo.feature.catalogo.resources.col_margin
import br.com.encomendo.feature.catalogo.resources.col_portal
import br.com.encomendo.feature.catalogo.resources.col_price
import br.com.encomendo.feature.catalogo.resources.col_source
import br.com.encomendo.feature.catalogo.resources.col_supplier
import br.com.encomendo.feature.catalogo.resources.items_archived
import br.com.encomendo.feature.catalogo.resources.items_open
import br.com.encomendo.feature.catalogo.resources.lead_days
import br.com.encomendo.feature.catalogo.resources.margin_none
import br.com.encomendo.feature.catalogo.resources.margin_value
import br.com.encomendo.feature.catalogo.resources.no_category
import br.com.encomendo.feature.catalogo.resources.portal_switch_of
import org.jetbrains.compose.resources.pluralStringResource
import org.jetbrains.compose.resources.stringResource

/**
 * ItemCard is one row of the web's table on a phone: name, "Arquivado", Categoria, Origem, Fornecedor, Prazo,
 * Preço, then Custo and Margem only when the API sent them, and the "No portal" switch. Tapping it opens the item.
 */
@Composable
internal fun ItemCard(
    summary: ItemSummary,
    state: ItemsUiState,
    actions: ItemsActions,
) {
    val open = stringResource(Res.string.items_open, summary.name)
    // The whole card opens the item; the click label says which, and the card's own text is read as it is.
    val cardModifier =
        Modifier
            .fillMaxWidth()
            .clickable(onClickLabel = open) { actions.navigation.onOpenItem(summary.id) }
    OutlinedCard(modifier = cardModifier) {
        Column(modifier = Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            summary.cover?.let { cover ->
                PhotoImage(
                    url = cover.url,
                    contentDescription = null,
                    modifier = Modifier.size(COVER_SIZE).clip(MaterialTheme.shapes.small),
                )
            }
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.CenterVertically) {
                Text(
                    text = summary.name,
                    style = MaterialTheme.typography.titleMedium,
                    modifier = Modifier.weight(1f, fill = false),
                )
                if (summary.archived) {
                    Text(
                        text = stringResource(Res.string.items_archived),
                        style = MaterialTheme.typography.labelLarge,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
            val category = state.categoryName(summary.categoryId) ?: stringResource(Res.string.no_category)
            ItemLine(stringResource(Res.string.col_category), category)
            ItemLine(stringResource(Res.string.col_source), summary.sourcingMode.label())
            summary.mainSupplierName?.let { ItemLine(stringResource(Res.string.col_supplier), it) }
            ItemLine(
                stringResource(Res.string.col_lead),
                pluralStringResource(Res.plurals.lead_days, summary.readyDays, summary.readyDays),
            )
            ItemLine(stringResource(Res.string.col_price), formatMoney(summary.price))
            ItemCosts(summary)
            PortalSwitch(summary = summary, state = state, onToggle = actions.onTogglePublished)
        }
    }
}

/** "Custo" and "Margem", which exist only when the API sent them: the owner's columns. */
@Composable
private fun ItemCosts(summary: ItemSummary) {
    if (summary.cost is OwnerField.Shown) {
        ItemLine(
            stringResource(Res.string.col_cost),
            summary.cost.valueOrNull?.let(::formatMoney) ?: stringResource(Res.string.margin_none),
        )
    }
    if (summary.margin is OwnerField.Shown) {
        val margin = summary.margin.valueOrNull
        val text =
            margin?.let {
                stringResource(
                    Res.string.margin_value,
                    formatMoney(Money(it.cents, summary.price.currency)),
                    formatBasisPoints(it.basisPoints),
                )
            } ?: stringResource(Res.string.margin_none)
        ItemLine(stringResource(Res.string.col_margin), text)
    }
}

/** "No portal": a switch, read aloud as "Mostrar {item} na página da loja". Only a writer can flip it. */
@Composable
private fun PortalSwitch(
    summary: ItemSummary,
    state: ItemsUiState,
    onToggle: (ItemSummary, Boolean) -> Unit,
) {
    val description = stringResource(Res.string.portal_switch_of, summary.name)
    val enabled = state.canWrite && !summary.archived && state.savingId == null
    // The whole row is the switch's touch target, and it is read out as the doc says, with its state.
    val rowModifier =
        Modifier
            .fillMaxWidth()
            .heightIn(min = TouchTarget)
            .toggleable(
                value = summary.published,
                enabled = enabled,
                role = Role.Switch,
                onValueChange = { onToggle(summary, it) },
            ).semantics { contentDescription = description }
    Row(
        modifier = rowModifier,
        horizontalArrangement = Arrangement.SpaceBetween,
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(text = stringResource(Res.string.col_portal), style = MaterialTheme.typography.bodyLarge)
        Switch(checked = summary.published, onCheckedChange = null, enabled = enabled)
    }
}

/** One labelled value of a card. */
@Composable
internal fun ItemLine(
    label: String,
    value: String,
) {
    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        Text(
            text = label,
            style = MaterialTheme.typography.bodyLarge,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Text(text = value, style = MaterialTheme.typography.bodyLarge)
    }
}

private val COVER_SIZE = 72.dp
