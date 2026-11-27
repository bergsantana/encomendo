package br.com.encomendo.feature.catalogo

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import br.com.encomendo.core.domain.catalog.Kit
import br.com.encomendo.feature.catalogo.resources.Res
import br.com.encomendo.feature.catalogo.resources.kit_price_above_single
import br.com.encomendo.feature.catalogo.resources.kit_total_days
import br.com.encomendo.feature.catalogo.resources.kit_total_discount
import br.com.encomendo.feature.catalogo.resources.kit_total_margin
import br.com.encomendo.feature.catalogo.resources.kit_total_single
import br.com.encomendo.feature.catalogo.resources.kit_totals_new
import br.com.encomendo.feature.catalogo.resources.kit_totals_stale
import br.com.encomendo.feature.catalogo.resources.kit_totals_title
import org.jetbrains.compose.resources.pluralStringResource
import org.jetbrains.compose.resources.stringResource

/**
 * KitTotals is the totals block of the kit editor. Every figure is the one the API sent with the kit as it was last
 * saved; nothing is added up here (ADR 0025). While the form holds edits the figures are stale, and the block says so.
 * "Margem" shows only when the API sent it, which is to the owner; "—" when the owner's kit has no cost.
 */
@Composable
internal fun KitTotals(state: KitFormUiState) {
    SectionTitle(stringResource(Res.string.kit_totals_title))
    val kit = state.kit
    if (kit == null) {
        Hint(stringResource(Res.string.kit_totals_new))
        return
    }
    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
        if (state.dirty) Hint(stringResource(Res.string.kit_totals_stale))
        TotalLine(stringResource(Res.string.kit_total_single, formatMoney(kit.singleItemsTotal)))
        TotalLine(stringResource(Res.string.kit_total_discount, formatMoney(kit.discount)))
        kit.margin.text(kit.price.currency)?.let { TotalLine(stringResource(Res.string.kit_total_margin, it)) }
        TotalLine(daysText(kit, state.niche.montagem))
        // The API's own discount decides: a negative one is a kit priced above its items. It warns and never blocks.
        if (state.priceAboveItems) {
            Text(
                text = stringResource(Res.string.kit_price_above_single),
                color = MaterialTheme.colorScheme.error,
                style = MaterialTheme.typography.bodyLarge,
                modifier = Modifier.announced(),
            )
        }
    }
}

/** "Prazo calculado: {n} dias (item mais lento {a} + {Montagem} {b} + margem de segurança {c})". */
@Composable
internal fun daysText(
    kit: Kit,
    montagem: String,
): String =
    pluralStringResource(
        Res.plurals.kit_total_days,
        daysQuantity(kit.days.total),
        kit.days.total,
        kit.days.slowestItem,
        montagem,
        kit.days.assembly,
        kit.days.safetyMargin,
    )

@Composable
private fun TotalLine(text: String) = Text(text = text, style = MaterialTheme.typography.bodyLarge)

@Composable
internal fun Hint(text: String) =
    Text(text = text, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
