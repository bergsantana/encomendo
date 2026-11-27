package br.com.encomendo.feature.catalogo

import br.com.encomendo.core.domain.catalog.Category
import br.com.encomendo.core.domain.catalog.ItemSummary

/** AlternativeGroup is one block of "Pode trocar por": a heading of its own, or none, and its items. */
internal data class AlternativeGroup(
    val heading: AlternativeHeading?,
    val items: List<ItemSummary>,
)

/** AlternativeHeading is what a group of "Pode trocar por" is called. */
internal sealed interface AlternativeHeading {
    /** "Sugestões da categoria {categoria}": the items of the row's own category. */
    data class Suggestions(
        val categoryName: String,
    ) : AlternativeHeading

    /** "Outros itens": the rest, listed after the suggestions. */
    data object Others : AlternativeHeading
}

/**
 * Lists what the row of [itemId] may be swapped for: the items of its own category first, under "Sugestões da
 * categoria {categoria}", then the rest under "Outros itens". Without a category the store still knows (none, or one
 * deleted meanwhile) there is nothing to suggest from, so one list with no heading comes back. An archived item stays
 * listed only while the row still names it, so it can be unticked.
 */
internal fun alternativesFor(
    itemId: String,
    picked: List<String>,
    catalogItems: List<ItemSummary>,
    categories: List<Category>,
): List<AlternativeGroup> {
    val own = catalogItems.firstOrNull { it.id == itemId }
    val candidates = catalogItems.filter { it.id != itemId && (!it.archived || it.id in picked) }
    val ownCategory = own?.categoryId
    val categoryName = categories.firstOrNull { it.id == ownCategory }?.name
    val suggested = if (categoryName == null) emptyList() else candidates.filter { it.categoryId == ownCategory }
    val others = candidates - suggested.toSet()
    val suggestions = categoryName?.takeIf { suggested.isNotEmpty() }?.let(AlternativeHeading::Suggestions)
    val othersHeading = if (suggestions == null) null else AlternativeHeading.Others
    return buildList {
        if (suggestions != null) add(AlternativeGroup(suggestions, suggested))
        if (others.isNotEmpty()) add(AlternativeGroup(othersHeading, others))
    }
}

/** The items "Item a adicionar" offers: not archived, and not in the kit yet. */
internal fun addableItems(
    catalogItems: List<ItemSummary>,
    inKit: List<KitItemFields>,
): List<ItemSummary> {
    val taken = inKit.map { it.itemId }.toSet()
    return catalogItems.filter { !it.archived && it.id !in taken }
}
