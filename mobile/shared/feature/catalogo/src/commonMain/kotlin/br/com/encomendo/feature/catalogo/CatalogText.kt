package br.com.encomendo.feature.catalogo

import androidx.compose.runtime.Composable
import br.com.encomendo.core.domain.catalog.SourcingMode
import br.com.encomendo.core.ui.failure.text
import br.com.encomendo.feature.catalogo.resources.Res
import br.com.encomendo.feature.catalogo.resources.items_offline_saved
import br.com.encomendo.feature.catalogo.resources.message_category_in_use
import br.com.encomendo.feature.catalogo.resources.message_check_fields
import br.com.encomendo.feature.catalogo.resources.message_conflict
import br.com.encomendo.feature.catalogo.resources.message_item_in_kit
import br.com.encomendo.feature.catalogo.resources.message_item_in_kits
import br.com.encomendo.feature.catalogo.resources.message_item_in_unnamed_kit
import br.com.encomendo.feature.catalogo.resources.message_kit_conflict
import br.com.encomendo.feature.catalogo.resources.message_kit_photo_limit
import br.com.encomendo.feature.catalogo.resources.message_photo_invalid
import br.com.encomendo.feature.catalogo.resources.message_photo_limit
import br.com.encomendo.feature.catalogo.resources.message_photo_not_uploaded
import br.com.encomendo.feature.catalogo.resources.message_photo_too_big
import br.com.encomendo.feature.catalogo.resources.notice_archived
import br.com.encomendo.feature.catalogo.resources.notice_restored
import br.com.encomendo.feature.catalogo.resources.notice_saved
import br.com.encomendo.feature.catalogo.resources.problem_backup_same
import br.com.encomendo.feature.catalogo.resources.problem_backup_supplier_missing
import br.com.encomendo.feature.catalogo.resources.problem_choice_days_invalid
import br.com.encomendo.feature.catalogo.resources.problem_choice_duplicate
import br.com.encomendo.feature.catalogo.resources.problem_choice_label_missing
import br.com.encomendo.feature.catalogo.resources.problem_choice_price_invalid
import br.com.encomendo.feature.catalogo.resources.problem_cost_invalid
import br.com.encomendo.feature.catalogo.resources.problem_kit_alternatives_missing
import br.com.encomendo.feature.catalogo.resources.problem_kit_days_range
import br.com.encomendo.feature.catalogo.resources.problem_kit_items_missing
import br.com.encomendo.feature.catalogo.resources.problem_kit_name_missing
import br.com.encomendo.feature.catalogo.resources.problem_kit_price_missing
import br.com.encomendo.feature.catalogo.resources.problem_kit_quantity_range
import br.com.encomendo.feature.catalogo.resources.problem_lead_days_missing
import br.com.encomendo.feature.catalogo.resources.problem_lead_days_range
import br.com.encomendo.feature.catalogo.resources.problem_main_supplier_missing
import br.com.encomendo.feature.catalogo.resources.problem_name_missing
import br.com.encomendo.feature.catalogo.resources.problem_option_name_missing
import br.com.encomendo.feature.catalogo.resources.problem_option_needs_choice
import br.com.encomendo.feature.catalogo.resources.problem_price_missing
import br.com.encomendo.feature.catalogo.resources.problem_production_days_missing
import br.com.encomendo.feature.catalogo.resources.problem_production_days_range
import br.com.encomendo.feature.catalogo.resources.source_in_house
import br.com.encomendo.feature.catalogo.resources.source_stock
import br.com.encomendo.feature.catalogo.resources.source_to_order
import kotlinx.datetime.LocalDateTime
import kotlinx.datetime.TimeZone
import kotlinx.datetime.format.char
import kotlinx.datetime.toLocalDateTime
import org.jetbrains.compose.resources.StringResource
import org.jetbrains.compose.resources.stringResource
import kotlin.time.Instant

/** The text of [this] message, in the design document's words for Catálogo. */
@Composable
internal fun CatalogMessage.text(): String =
    when (this) {
        CatalogMessage.CheckFields -> stringResource(Res.string.message_check_fields)
        CatalogMessage.Conflict -> stringResource(Res.string.message_conflict)
        CatalogMessage.KitConflict -> stringResource(Res.string.message_kit_conflict)
        is CatalogMessage.KitItemsArchived -> kitItemsArchivedText(items)
        is CatalogMessage.ItemInKit -> inKitText(kits)
        CatalogMessage.CategoryInUse -> stringResource(Res.string.message_category_in_use)
        CatalogMessage.PhotoTooBig -> stringResource(Res.string.message_photo_too_big)
        CatalogMessage.PhotoInvalid -> stringResource(Res.string.message_photo_invalid)
        CatalogMessage.PhotoLimit -> stringResource(Res.string.message_photo_limit)
        CatalogMessage.KitPhotoLimit -> stringResource(Res.string.message_kit_photo_limit)
        CatalogMessage.PhotoNotUploaded -> stringResource(Res.string.message_photo_not_uploaded)
        is CatalogMessage.General -> message.text()
    }

/**
 * What archiving refused says about the kits that hold the item: none named, one, or several joined as "A, B e C".
 * The API names every kit that holds it, but an item archived in a kit it does not name still gets an answer.
 */
@Composable
private fun inKitText(kits: List<String>): String =
    when (kits.size) {
        0 -> stringResource(Res.string.message_item_in_unnamed_kit)
        1 -> stringResource(Res.string.message_item_in_kit, kits.single())
        else -> stringResource(Res.string.message_item_in_kits, kits.joinedWithE())
    }

/** The status line for [this] change. */
@Composable
internal fun ItemNotice.noticeText(): String =
    when (this) {
        CatalogChange.ItemSaved -> stringResource(Res.string.notice_saved)
        is CatalogChange.ItemArchived -> stringResource(Res.string.notice_archived, name)
        is CatalogChange.ItemRestored -> stringResource(Res.string.notice_restored, name)
    }

/** The text under a form field for [this] issue. */
@Composable
internal fun FieldIssue.text(): String =
    when (this) {
        is FieldIssue.Server -> message
        is FieldIssue.Known -> problem.text()
    }

@Composable
private fun FieldProblem.text(): String = stringResource(problemTexts.getValue(this))

/** The text of each [FieldProblem]; a test checks that none is missing. */
internal val problemTexts: Map<FieldProblem, StringResource> =
    mapOf(
        FieldProblem.NameMissing to Res.string.problem_name_missing,
        FieldProblem.PriceMissing to Res.string.problem_price_missing,
        FieldProblem.MainSupplierMissing to Res.string.problem_main_supplier_missing,
        FieldProblem.BackupSupplierMissing to Res.string.problem_backup_supplier_missing,
        FieldProblem.BackupSameAsMain to Res.string.problem_backup_same,
        FieldProblem.LeadDaysMissing to Res.string.problem_lead_days_missing,
        FieldProblem.LeadDaysRange to Res.string.problem_lead_days_range,
        FieldProblem.CostInvalid to Res.string.problem_cost_invalid,
        FieldProblem.ProductionDaysMissing to Res.string.problem_production_days_missing,
        FieldProblem.ProductionDaysRange to Res.string.problem_production_days_range,
        FieldProblem.OptionNameMissing to Res.string.problem_option_name_missing,
        FieldProblem.OptionNeedsChoice to Res.string.problem_option_needs_choice,
        FieldProblem.ChoiceLabelMissing to Res.string.problem_choice_label_missing,
        FieldProblem.ChoiceDuplicate to Res.string.problem_choice_duplicate,
        FieldProblem.ChoicePriceInvalid to Res.string.problem_choice_price_invalid,
        FieldProblem.ChoiceDaysInvalid to Res.string.problem_choice_days_invalid,
        FieldProblem.KitNameMissing to Res.string.problem_kit_name_missing,
        FieldProblem.KitPriceMissing to Res.string.problem_kit_price_missing,
        FieldProblem.KitItemsMissing to Res.string.problem_kit_items_missing,
        FieldProblem.KitQuantityRange to Res.string.problem_kit_quantity_range,
        FieldProblem.KitAlternativesMissing to Res.string.problem_kit_alternatives_missing,
        FieldProblem.KitDaysRange to Res.string.problem_kit_days_range,
    )

/** "Origem": Sob encomenda, Estoque or Produção própria. */
@Composable
internal fun SourcingMode.label(): String =
    when (this) {
        SourcingMode.ToOrder -> stringResource(Res.string.source_to_order)
        SourcingMode.Stock -> stringResource(Res.string.source_stock)
        SourcingMode.InHouse -> stringResource(Res.string.source_in_house)
    }

/** "Mostrando o catálogo salvo neste aparelho em {data}." for a copy saved at [epochMillis]. */
@Composable
internal fun offlineSavedText(epochMillis: Long): String =
    stringResource(Res.string.items_offline_saved, ptBrDateTime(Instant.fromEpochMilliseconds(epochMillis)))

/** Returns [instant] as "dd/mm/aaaa hh:mm", in the phone's time zone. Only formats a moment the device kept. */
internal fun ptBrDateTime(
    instant: Instant,
    zone: TimeZone = TimeZone.currentSystemDefault(),
): String = PT_BR_DATE_TIME.format(instant.toLocalDateTime(zone))

private val PT_BR_DATE_TIME =
    LocalDateTime.Format {
        day()
        char('/')
        monthNumber()
        char('/')
        year()
        char(' ')
        hour()
        char(':')
        minute()
    }
