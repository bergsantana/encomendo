package br.com.encomendo.feature.catalogo

import br.com.encomendo.core.domain.catalog.CostChange
import br.com.encomendo.core.domain.catalog.ItemDraft
import br.com.encomendo.core.domain.catalog.OptionChoiceDraft
import br.com.encomendo.core.domain.catalog.OptionDraft
import br.com.encomendo.core.domain.catalog.SourcingMode
import br.com.encomendo.core.domain.catalog.SupplierLinkDraft

/** ItemField names one field of the item form, so a problem is marked next to it. */
sealed interface ItemField {
    data object Name : ItemField

    data object Price : ItemField

    /** The supplier choice of [slot]. */
    data class Supplier(
        val slot: SupplierSlot,
    ) : ItemField

    /** The lead days of [slot]. */
    data class LeadDays(
        val slot: SupplierSlot,
    ) : ItemField

    /** The cost of [slot]. */
    data class Cost(
        val slot: SupplierSlot,
    ) : ItemField

    data object ProductionDays : ItemField

    data object ProductionCost : ItemField

    /** The name of the option [key]. */
    data class OptionName(
        val key: Int,
    ) : ItemField

    /** The list of choices of the option [key], which needs at least one. */
    data class OptionChoices(
        val key: Int,
    ) : ItemField

    /** The label of one choice. */
    data class ChoiceLabel(
        val option: Int,
        val choice: Int,
    ) : ItemField

    /** The "Acréscimo no preço" of one choice. */
    data class ChoicePrice(
        val option: Int,
        val choice: Int,
    ) : ItemField

    /** The "Dias a mais" of one choice. */
    data class ChoiceDays(
        val option: Int,
        val choice: Int,
    ) : ItemField
}

/** SupplierSlot says which supplier of the item a field is about. */
enum class SupplierSlot {
    /** "Fornecedor principal". */
    Main,

    /** "Fornecedor reserva". */
    Backup,
}

/** FieldProblem is what the design document says is wrong with a field, one case for each text. */
enum class FieldProblem {
    NameMissing,
    PriceMissing,
    MainSupplierMissing,
    BackupSupplierMissing,
    BackupSameAsMain,
    LeadDaysMissing,
    LeadDaysRange,
    CostInvalid,
    ProductionDaysMissing,
    OptionNameMissing,
    OptionNeedsChoice,
    ChoiceLabelMissing,
    ChoiceDuplicate,
    ChoicePriceInvalid,
    ChoiceDaysInvalid,
}

/** FieldIssue is a problem marked next to a field: the client's own check, or the server's words. */
sealed interface FieldIssue {
    /** A check the form made before sending. */
    data class Known(
        val problem: FieldProblem,
    ) : FieldIssue

    /** What the API refused, in its own pt-BR words. */
    data class Server(
        val message: String,
    ) : FieldIssue {
        // The server's words may quote what was typed.
        override fun toString(): String = "Server"
    }
}

/** DraftResult is the form read into what the API takes, or the fields that cannot be read. */
internal sealed interface DraftResult {
    /** Every field reads: [draft] is ready to send. */
    data class Ready(
        val draft: ItemDraft,
    ) : DraftResult

    /** Some fields do not read; each is marked. */
    data class Problems(
        val issues: Map<ItemField, FieldIssue>,
    ) : DraftResult
}

private const val MAX_DAYS = 365

/**
 * Reads the form into an [ItemDraft], marking every field that does not read, as the web does. A cost the owner did
 * not touch is [CostChange.Keep], and anyone who does not see costs ([seesCosts] false) never sends one (ADR 0025).
 */
internal fun ItemFields.toDraft(seesCosts: Boolean): DraftResult {
    val check = Check(seesCosts)
    val draft = draftOrNull(check)
    return if (draft == null ||
        check.issues.isNotEmpty()
    ) {
        DraftResult.Problems(check.issues)
    } else {
        DraftResult.Ready(draft)
    }
}

/** The draft when the name, price and sourcing read; null otherwise. Options mark their own problems in [check]. */
private fun ItemFields.draftOrNull(check: Check): ItemDraft? {
    val itemName = check.require(name.trim().takeIf { it.isNotEmpty() }, ItemField.Name, FieldProblem.NameMissing)
    val cents = check.require(parseCents(price)?.takeIf { it > 0 }, ItemField.Price, FieldProblem.PriceMissing)
    val sourcing = sourcingOf(check)
    val optionDrafts = options.map { it.toDraft(check) }
    if (itemName == null || cents == null || sourcing == null) return null
    return ItemDraft(
        name = itemName,
        description = description.trim(),
        categoryId = categoryId,
        sourcingMode = mode,
        priceCents = cents,
        productionDays = sourcing.productionDays,
        productionCost = sourcing.productionCost,
        mainSupplier = sourcing.main,
        backupSupplier = sourcing.backup,
        attributeValueIds = attributeValueIds.toList(),
        options = optionDrafts.filterNotNull(),
        published = published,
    )
}

/** What the sourcing part of the form says: production, or the main and backup suppliers. */
private class SourcingDraft(
    val productionDays: Int?,
    val productionCost: CostChange,
    val main: SupplierLinkDraft?,
    val backup: SupplierLinkDraft?,
)

private fun ItemFields.sourcingOf(check: Check): SourcingDraft? =
    if (mode == SourcingMode.InHouse) {
        val days =
            check.require(
                productionDays.trim().toIntOrNull()?.takeIf { it in 1..MAX_DAYS },
                ItemField.ProductionDays,
                FieldProblem.ProductionDaysMissing,
            )
        val cost = check.cost(productionCost, productionCostTouched, ItemField.ProductionCost)
        if (days == null || cost == null) null else SourcingDraft(days, cost, main = null, backup = null)
    } else {
        val mainLink = check.link(main, SupplierSlot.Main)
        val backupLink = backup?.let { check.link(it, SupplierSlot.Backup) }
        if (backup != null && backup.supplierId != null && backup.supplierId == main.supplierId) {
            check.issues[ItemField.Supplier(SupplierSlot.Backup)] = FieldIssue.Known(FieldProblem.BackupSameAsMain)
        }
        // A backup that does not read is already marked; the whole form stays unsent.
        if (mainLink == null ||
            (backup != null && backupLink == null)
        ) {
            null
        } else {
            SourcingDraft(null, CostChange.Keep, mainLink, backupLink)
        }
    }

private fun OptionFields.toDraft(check: Check): OptionDraft? {
    val optionName =
        check.require(name.trim().takeIf { it.isNotEmpty() }, ItemField.OptionName(key), FieldProblem.OptionNameMissing)
    if (choices.isEmpty()) check.mark(ItemField.OptionChoices(key), FieldProblem.OptionNeedsChoice)
    val seen = mutableSetOf<String>()
    val drafts =
        choices.map { choice ->
            val draft = choice.toDraft(key, check)
            if (draft != null && !seen.add(draft.label.lowercase())) {
                check.mark(ItemField.ChoiceLabel(key, choice.key), FieldProblem.ChoiceDuplicate)
            }
            draft
        }
    return if (optionName == null || choices.isEmpty() || drafts.any { it == null }) {
        null
    } else {
        OptionDraft(id, optionName, required, drafts.filterNotNull())
    }
}

private fun ChoiceFields.toDraft(
    optionKey: Int,
    check: Check,
): OptionChoiceDraft? {
    val text =
        check.require(
            label.trim().takeIf {
                it.isNotEmpty()
            },
            ItemField.ChoiceLabel(optionKey, key),
            FieldProblem.ChoiceLabelMissing,
        )
    val price =
        if (extraPrice.isBlank()) 0L else parseCents(extraPrice)
    val days = if (extraDays.isBlank()) 0 else extraDays.trim().toIntOrNull()?.takeIf { it in 0..MAX_DAYS }
    if (price == null) check.mark(ItemField.ChoicePrice(optionKey, key), FieldProblem.ChoicePriceInvalid)
    if (days == null) check.mark(ItemField.ChoiceDays(optionKey, key), FieldProblem.ChoiceDaysInvalid)
    return if (text == null || price == null || days == null) null else OptionChoiceDraft(id, text, price, days)
}

/** The problems found so far while reading the form, and what the role may send. */
private class Check(
    val seesCosts: Boolean,
) {
    val issues = LinkedHashMap<ItemField, FieldIssue>()

    fun mark(
        field: ItemField,
        problem: FieldProblem,
    ) {
        issues[field] = FieldIssue.Known(problem)
    }

    fun <T> require(
        value: T?,
        field: ItemField,
        problem: FieldProblem,
    ): T? {
        if (value == null) mark(field, problem)
        return value
    }

    /** Reads one cost field: left alone unless the owner touched it, then cleared or set. */
    fun cost(
        text: String,
        touched: Boolean,
        field: ItemField,
    ): CostChange? =
        when {
            !seesCosts || !touched -> CostChange.Keep
            text.isBlank() -> CostChange.Clear
            else -> require(parseCents(text)?.let(CostChange::Set), field, FieldProblem.CostInvalid)
        }

    /** Reads the supplier of [slot]; null when it does not read, with each problem marked. */
    fun link(
        fields: SupplierFields,
        slot: SupplierSlot,
    ): SupplierLinkDraft? {
        val missing =
            if (slot ==
                SupplierSlot.Main
            ) {
                FieldProblem.MainSupplierMissing
            } else {
                FieldProblem.BackupSupplierMissing
            }
        val id = require(fields.supplierId, ItemField.Supplier(slot), missing)
        val days = leadDays(fields.leadDays, slot)
        val cost = cost(fields.cost, fields.costTouched, ItemField.Cost(slot))
        return if (id == null || days == null || cost == null) null else SupplierLinkDraft(id, days, cost)
    }

    private fun leadDays(
        text: String,
        slot: SupplierSlot,
    ): Int? {
        val field = ItemField.LeadDays(slot)
        if (text.isBlank()) return require(null, field, FieldProblem.LeadDaysMissing)
        return require(text.trim().toIntOrNull()?.takeIf { it in 0..MAX_DAYS }, field, FieldProblem.LeadDaysRange)
    }
}
