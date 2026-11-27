package br.com.encomendo.feature.catalogo

import androidx.compose.runtime.Composable
import br.com.encomendo.feature.catalogo.resources.Res
import br.com.encomendo.feature.catalogo.resources.message_conflict
import br.com.encomendo.feature.catalogo.resources.message_kit_conflict
import br.com.encomendo.feature.catalogo.resources.message_kit_item_archived
import br.com.encomendo.feature.catalogo.resources.message_kit_item_archived_unnamed
import br.com.encomendo.feature.catalogo.resources.message_kit_photo_limit
import br.com.encomendo.feature.catalogo.resources.message_photo_limit
import br.com.encomendo.feature.catalogo.resources.notice_archived
import br.com.encomendo.feature.catalogo.resources.notice_restored
import org.jetbrains.compose.resources.stringResource

// The sentences that differ between an item and a kit, in the design document's words for Catálogo.

/** "Outra pessoa alterou este item…", or "…este kit…" on the kit editor. */
@Composable
internal fun conflictText(): String =
    stringResource(
        if (LocalCatalogSubject.current ==
            CatalogSubject.Kit
        ) {
            Res.string.message_kit_conflict
        } else {
            Res.string.message_conflict
        },
    )

/** "Cada item pode ter até 8 fotos.", or "Cada kit…" on the kit editor. */
@Composable
internal fun photoLimitText(): String =
    stringResource(
        if (LocalCatalogSubject.current == CatalogSubject.Kit) {
            Res.string.message_kit_photo_limit
        } else {
            Res.string.message_photo_limit
        },
    )

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
