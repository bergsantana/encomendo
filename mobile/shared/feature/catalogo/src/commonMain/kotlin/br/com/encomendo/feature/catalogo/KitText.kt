package br.com.encomendo.feature.catalogo

import androidx.compose.runtime.Composable
import br.com.encomendo.feature.catalogo.resources.Res
import br.com.encomendo.feature.catalogo.resources.kit_store_default
import br.com.encomendo.feature.catalogo.resources.lead_days
import br.com.encomendo.feature.catalogo.resources.message_kit_item_archived
import br.com.encomendo.feature.catalogo.resources.message_kit_item_archived_unnamed
import br.com.encomendo.feature.catalogo.resources.notice_archived
import br.com.encomendo.feature.catalogo.resources.notice_restored
import org.jetbrains.compose.resources.pluralStringResource
import org.jetbrains.compose.resources.stringResource

// Kit sentences and the small helpers the kit screens share, in the design document's words for Catálogo.

/**
 * The plural quantity for [days]: 1 for "1 dia", 2 (the "other" form) for everything else, 0 included. Portuguese
 * plural rules put 0 with the singular, but the doc says "0 dias", as the web's `formatDays` does.
 */
internal fun daysQuantity(days: Int): Int = if (days == 1) 1 else 2

/** "Padrão da loja: 1 dia" or "Padrão da loja: {n} dias". */
@Composable
internal fun defaultDaysHint(days: Int): String =
    pluralStringResource(Res.plurals.kit_store_default, daysQuantity(days), days)

/** "{n} dias" ("1 dia", "0 dias"). */
@Composable
internal fun daysLabel(days: Int): String = pluralStringResource(Res.plurals.lead_days, daysQuantity(days), days)

/** An archived item the API names, said once per item; with none named, the general sentence. */
@Composable
internal fun kitItemsArchivedText(items: List<String>): String =
    if (items.isEmpty()) {
        stringResource(Res.string.message_kit_item_archived_unnamed)
    } else {
        items.map { stringResource(Res.string.message_kit_item_archived, it) }.joinToString(" ")
    }

/** The status line for [this] kit change. */
@Composable
internal fun KitNotice.noticeText(): String =
    when (this) {
        is CatalogChange.KitArchived -> stringResource(Res.string.notice_archived, name)
        is CatalogChange.KitRestored -> stringResource(Res.string.notice_restored, name)
    }
