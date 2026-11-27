package br.com.encomendo.feature.catalogo

import androidx.compose.runtime.Composable
import br.com.encomendo.core.domain.catalog.Margin
import br.com.encomendo.core.domain.catalog.OwnerField
import br.com.encomendo.core.domain.catalog.valueOrNull
import br.com.encomendo.core.domain.money.Money
import br.com.encomendo.feature.catalogo.resources.Res
import br.com.encomendo.feature.catalogo.resources.margin_none
import br.com.encomendo.feature.catalogo.resources.margin_value
import org.jetbrains.compose.resources.stringResource

/**
 * The text of a margin the API sent: "{valor} ({n}%)", or "—" without a cost to take it from. Null when the API left
 * the margin out, which is to every role but the owner: no line shows then. Formatting only; the server computed it.
 */
@Composable
internal fun OwnerField<Margin>.text(currency: String): String? {
    if (this !is OwnerField.Shown) return null
    val margin = valueOrNull
    return if (margin == null) {
        stringResource(Res.string.margin_none)
    } else {
        stringResource(
            Res.string.margin_value,
            formatMoney(Money(margin.cents, currency)),
            formatBasisPoints(margin.basisPoints),
        )
    }
}
