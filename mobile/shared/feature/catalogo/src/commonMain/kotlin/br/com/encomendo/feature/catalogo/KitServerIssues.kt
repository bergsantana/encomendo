package br.com.encomendo.feature.catalogo

import br.com.encomendo.core.domain.failure.InputIssue

/**
 * Maps the fields an API refusal names onto the kit editor's fields, so each of the server's own words shows next to
 * its field. The API names fields as the request does (`priceCents`, `items[0].quantity`,
 * `items[1].alternativeItemIds[0]`); a field the editor has no place for stays in [unplaced].
 */
internal class KitServerIssues(
    /** The refused fields the editor could mark, with the server's words. */
    val placed: Map<KitField, FieldIssue>,
    /** The refusals about fields the editor could not place. */
    val unplaced: List<InputIssue>,
)

private val ROW_PATH = Regex("""^items\[(\d+)](?:\.(quantity|itemId|swappable|alternativeItemIds)(?:\[\d+])?)?$""")
private const val INDEX_GROUP = 1
private const val PART_GROUP = 2

/** Reads [issues] against the editor as it was sent: row indexes are positions in [fields]. */
internal fun placeKitIssues(
    issues: List<InputIssue>,
    fields: KitFields,
): KitServerIssues {
    val placed = LinkedHashMap<KitField, FieldIssue>()
    val unplaced = mutableListOf<InputIssue>()
    issues.forEach { issue ->
        val field = kitFieldNamed(issue.field, fields)
        if (field == null) unplaced += issue else placed.putIfAbsent(field, FieldIssue.Server(issue.message))
    }
    return KitServerIssues(placed, unplaced)
}

private fun kitFieldNamed(
    name: String,
    fields: KitFields,
): KitField? =
    when (name) {
        "name" -> KitField.Name
        "priceCents", "price" -> KitField.Price
        "assemblyDays" -> KitField.AssemblyDays
        "safetyMarginDays" -> KitField.SafetyMarginDays
        "items" -> KitField.Items
        else -> rowField(name, fields)
    }

private fun rowField(
    name: String,
    fields: KitFields,
): KitField? {
    val match = ROW_PATH.matchEntire(name)
    val row =
        match
            ?.groupValues
            ?.get(INDEX_GROUP)
            ?.toIntOrNull()
            ?.let(fields.items::getOrNull)
    if (match == null || row == null) return null
    return when (match.groupValues[PART_GROUP]) {
        "quantity" -> KitField.Quantity(row.itemId)
        "alternativeItemIds" -> KitField.Alternatives(row.itemId)
        // The row itself (its item, or whether it swaps) has no field of its own: its sentence shows on "Itens do kit".
        else -> KitField.Items
    }
}
