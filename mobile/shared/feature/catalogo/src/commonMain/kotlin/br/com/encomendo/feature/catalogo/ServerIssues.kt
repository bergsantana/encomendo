package br.com.encomendo.feature.catalogo

import br.com.encomendo.core.domain.failure.InputIssue

/**
 * Maps the fields an API refusal names onto the form's fields, so each of the server's own words shows next to its
 * field. The API names fields as the request does (`name`, `mainSupplier.leadDays`, `options[0].choices[1].label`).
 * An issue about a field the form has no place for stays in [unplaced] and shows above the form.
 */
internal class ServerIssues(
    /** The refused fields the form could mark, with the server's words. */
    val placed: Map<ItemField, FieldIssue>,
    /** The refusals about fields the form could not place. */
    val unplaced: List<InputIssue>,
)

/** Reads [issues] against the form as it was sent: option and choice indexes are positions in [fields]. */
internal fun placeServerIssues(
    issues: List<InputIssue>,
    fields: ItemFields,
): ServerIssues {
    val placed = LinkedHashMap<ItemField, FieldIssue>()
    val unplaced = mutableListOf<InputIssue>()
    issues.forEach { issue ->
        val field = fieldNamed(issue.field, fields)
        if (field == null) unplaced += issue else placed.putIfAbsent(field, FieldIssue.Server(issue.message))
    }
    return ServerIssues(placed, unplaced)
}

private val OPTION_PATH =
    Regex("""^options\[(\d+)](?:\.(name|choices)(?:\[(\d+)](?:\.(label|extraPriceCents|extraDays))?)?)?$""")
private val SUPPLIER_PATH = Regex("""^(mainSupplier|backupSupplier)(?:\.(supplierId|leadDays|costCents))?$""")

private fun fieldNamed(
    name: String,
    fields: ItemFields,
): ItemField? =
    when (name) {
        "name" -> ItemField.Name
        "priceCents", "price" -> ItemField.Price
        "productionDays" -> ItemField.ProductionDays
        "productionCostCents" -> ItemField.ProductionCost
        "leadDays" -> ItemField.LeadDays(SupplierSlot.Main)
        "costCents" -> ItemField.Cost(SupplierSlot.Main)
        else -> supplierField(name) ?: optionField(name, fields)
    }

private fun supplierField(name: String): ItemField? {
    val match = SUPPLIER_PATH.matchEntire(name) ?: return null
    val slot = if (match.groupValues[1] == "mainSupplier") SupplierSlot.Main else SupplierSlot.Backup
    return when (match.groupValues[2]) {
        "leadDays" -> ItemField.LeadDays(slot)
        "costCents" -> ItemField.Cost(slot)
        else -> ItemField.Supplier(slot)
    }
}

private const val OPTION_GROUP = 1
private const val PART_GROUP = 2
private const val CHOICE_GROUP = 3
private const val LEAF_GROUP = 4

private fun optionField(
    name: String,
    fields: ItemFields,
): ItemField? {
    val match = OPTION_PATH.matchEntire(name)
    val option =
        match
            ?.groupValues
            ?.get(OPTION_GROUP)
            ?.toIntOrNull()
            ?.let(fields.options::getOrNull)
    return if (match == null || option == null) null else optionPart(match, option)
}

private fun optionPart(
    match: MatchResult,
    option: OptionFields,
): ItemField? {
    val choiceIndex = match.groupValues[CHOICE_GROUP].toIntOrNull()
    val choice = choiceIndex?.let(option.choices::getOrNull)
    return when {
        match.groupValues[PART_GROUP] == "choices" && choiceIndex == null -> ItemField.OptionChoices(option.key)
        choiceIndex != null && choice == null -> null
        choice == null -> ItemField.OptionName(option.key)
        match.groupValues[LEAF_GROUP] == "extraPriceCents" -> ItemField.ChoicePrice(option.key, choice.key)
        match.groupValues[LEAF_GROUP] == "extraDays" -> ItemField.ChoiceDays(option.key, choice.key)
        else -> ItemField.ChoiceLabel(option.key, choice.key)
    }
}
