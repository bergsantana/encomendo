package br.com.encomendo.feature.catalogo

import br.com.encomendo.core.domain.catalog.Kit
import br.com.encomendo.core.domain.catalog.KitDraft
import br.com.encomendo.core.domain.catalog.KitItemDraft
import br.com.encomendo.core.domain.catalog.SwapDiscount

/** KitField names one field of the kit editor, so a problem is marked next to it. */
sealed interface KitField {
    data object Name : KitField

    data object Price : KitField

    /** "Itens do kit" as a whole, which needs at least one item. */
    data object Items : KitField

    data object AssemblyDays : KitField

    data object SafetyMarginDays : KitField

    /** The "Quantidade" of the row of [itemId]. */
    data class Quantity(
        val itemId: String,
    ) : KitField

    /** The "Pode trocar por" choices of the row of [itemId]. */
    data class Alternatives(
        val itemId: String,
    ) : KitField
}

/**
 * KitItemFields is one row of "Itens do kit" as typed. The item's id is the row's key: an item is in a kit once.
 */
data class KitItemFields(
    val itemId: String,
    val itemName: String,
    /** "Quantidade", as typed. */
    val quantity: String = "1",
    /** "Cliente pode trocar". */
    val swappable: Boolean = false,
    /** "Pode trocar por"; sent only while [swappable]. */
    val alternativeItemIds: List<String> = emptyList(),
    /** The item was archived after the kit was saved: the API refuses the kit until the row is taken out. */
    val archived: Boolean = false,
)

/**
 * KitFields is the kit editor as the user typed it. Text stays text until "Salvar". It holds no totals, discount or
 * days: those are the API's, shown from the kit as last saved, and never worked out here (ADR 0025).
 */
data class KitFields(
    val name: String = "",
    val description: String = "",
    /** "Preço do kit", as typed. */
    val price: String = "",
    /** "Ao trocar um item"; a new kit keeps the discount (ADR 0023). */
    val swapDiscount: SwapDiscount = SwapDiscount.Keep,
    /** "Dias de {Montagem}", as typed; blank uses the store's default. */
    val assemblyDays: String = "",
    /** "Margem de segurança (dias)", as typed; blank uses the store's default. */
    val safetyMarginDays: String = "",
    val items: List<KitItemFields> = emptyList(),
    val attributeValueIds: Set<String> = emptySet(),
    /** "Mostrar na página da loja". */
    val published: Boolean = false,
) {
    /** Returns the fields with the row of [itemId] set by [change]. */
    fun withRow(
        itemId: String,
        change: (KitItemFields) -> KitItemFields,
    ): KitFields = copy(items = items.map { if (it.itemId == itemId) change(it) else it })

    /** Returns the fields with the row of [itemId] swappable or not; its alternatives are kept for a change of mind. */
    fun withSwappable(
        itemId: String,
        swappable: Boolean,
    ): KitFields = withRow(itemId) { it.copy(swappable = swappable) }

    /** Returns the fields with [alternativeId] ticked or not under the row of [itemId]. */
    fun withAlternative(
        itemId: String,
        alternativeId: String,
        picked: Boolean,
    ): KitFields =
        withRow(itemId) { row ->
            val others = row.alternativeItemIds - alternativeId
            row.copy(alternativeItemIds = if (picked) others + alternativeId else others)
        }

    /** Returns the fields without the row of [itemId] ("Remover do kit"). */
    fun withoutRow(itemId: String): KitFields = copy(items = items.filterNot { it.itemId == itemId })

    /** Returns the fields with the field value [valueId] picked or not picked. */
    fun withValuePicked(
        valueId: String,
        picked: Boolean,
    ): KitFields = copy(attributeValueIds = if (picked) attributeValueIds + valueId else attributeValueIds - valueId)
}

/** Returns the fields the editor of [this] kit starts with; days are blank where the kit has no value of its own. */
internal fun Kit.toFields(): KitFields =
    KitFields(
        name = name,
        description = description,
        price = moneyInputText(price),
        swapDiscount = swapDiscount,
        assemblyDays = assemblyDays?.toString().orEmpty(),
        safetyMarginDays = safetyMarginDays?.toString().orEmpty(),
        items =
            items.map {
                KitItemFields(
                    it.itemId,
                    it.itemName,
                    it.quantity.toString(),
                    it.swappable,
                    it.alternativeItemIds,
                    it.archived,
                )
            },
        attributeValueIds = attributeValueIds.toSet(),
        published = published,
    )

/** KitDraftResult is the editor read into what the API takes, or the fields that cannot be read. */
internal sealed interface KitDraftResult {
    /** Every field reads: [draft] is ready to send. */
    data class Ready(
        val draft: KitDraft,
    ) : KitDraftResult

    /** Some fields do not read; each is marked. */
    data class Problems(
        val issues: Map<KitField, FieldIssue>,
    ) : KitDraftResult
}

private const val MAX_DAYS = 60
private const val MIN_QUANTITY = 1
private const val MAX_QUANTITY = 99

/**
 * Reads the editor into a [KitDraft], marking every field that does not read, with the web's checks. Blank days are
 * `null` (the store's default); a number is the kit's own. A row that is not swappable sends no alternatives.
 */
internal fun KitFields.toDraft(): KitDraftResult {
    val issues = LinkedHashMap<KitField, FieldIssue>()

    fun mark(
        field: KitField,
        problem: FieldProblem,
    ) {
        issues[field] = FieldIssue.Known(problem)
    }

    val cents = parseCents(price)?.takeIf { it > 0 }
    if (name.isBlank()) mark(KitField.Name, FieldProblem.KitNameMissing)
    if (cents == null) mark(KitField.Price, FieldProblem.KitPriceMissing)
    if (items.isEmpty()) mark(KitField.Items, FieldProblem.KitItemsMissing)
    val assembly = optionalDays(assemblyDays) { mark(KitField.AssemblyDays, FieldProblem.KitDaysRange) }
    val margin = optionalDays(safetyMarginDays) { mark(KitField.SafetyMarginDays, FieldProblem.KitDaysRange) }
    val rows = items.map { it.toDraft(::mark) }
    if (issues.isNotEmpty() || cents == null) return KitDraftResult.Problems(issues)
    val draft =
        KitDraft(
            name = name.trim(),
            description = description.trim(),
            priceCents = cents,
            swapDiscount = swapDiscount,
            assemblyDays = assembly,
            safetyMarginDays = margin,
            items = rows.filterNotNull(),
            attributeValueIds = attributeValueIds.toList(),
            published = published,
        )
    return KitDraftResult.Ready(draft)
}

private fun KitItemFields.toDraft(mark: (KitField, FieldProblem) -> Unit): KitItemDraft? {
    val count = quantity.trim().toIntOrNull()?.takeIf { it in MIN_QUANTITY..MAX_QUANTITY }
    if (count == null) mark(KitField.Quantity(itemId), FieldProblem.KitQuantityRange)
    if (swappable &&
        alternativeItemIds.isEmpty()
    ) {
        mark(KitField.Alternatives(itemId), FieldProblem.KitAlternativesMissing)
    }
    return count?.let { KitItemDraft(itemId, it, swappable, if (swappable) alternativeItemIds else emptyList()) }
}

/** Blank is `null`, the store's default; a whole number from 0 to 60 is the kit's own; anything else calls [wrong]. */
private fun optionalDays(
    text: String,
    wrong: () -> Unit,
): Int? {
    if (text.isBlank()) return null
    val days = text.trim().toIntOrNull()?.takeIf { it in 0..MAX_DAYS }
    if (days == null) wrong()
    return days
}
