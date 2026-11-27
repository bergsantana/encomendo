package br.com.encomendo.feature.catalogo

import br.com.encomendo.core.domain.catalog.Attribute
import br.com.encomendo.core.domain.catalog.Category
import br.com.encomendo.core.domain.catalog.ItemSummary
import br.com.encomendo.core.domain.catalog.Kit
import br.com.encomendo.core.domain.niche.NicheWords

/**
 * KitFormUiState is the kit editor ("Novo kit", "Editar kit"): what was typed, the problems marked next to the
 * fields, the choices to pick from, and what the role may do. A role without `catalog:write`, or an editor whose
 * store settings or items could not be read offline, gets the same state with [canWrite] false, and the screen
 * then shows the kit read-only.
 *
 * The totals, discount, margin and days are those of [kit], the kit as the API last returned it; nothing here works
 * any of them out.
 */
data class KitFormUiState(
    /** The kit to open, or null for "Novo kit". */
    val kitId: String? = null,
    /** The kit and the choices are being read. */
    val loading: Boolean = true,
    /** Why the kit could not be read: the message and "Tentar de novo" show instead of the form. */
    val loadMessage: CatalogMessage? = null,
    /** When the kit shown was saved on the device, in epoch milliseconds, if the server did not answer. */
    val offlineSince: Long? = null,
    /** The kit as last read or saved; null while it was never saved. Its totals and days are the API's. */
    val kit: Kit? = null,
    /** What was typed. */
    val fields: KitFields = KitFields(),
    /** What the editor started with or was last saved with, to tell whether anything was typed since. */
    val initialFields: KitFields = KitFields(),
    /** The problems marked next to their fields. */
    val issues: Map<KitField, FieldIssue> = emptyMap(),
    /** Why the last change failed, above the form. */
    val message: CatalogMessage? = null,
    /** "Kit salvo.", after a save: the editor stays open on the saved kit. */
    val saved: Boolean = false,
    /** Every item of the store, archived ones too, so a row can name and place its item. */
    val catalogItems: List<ItemSummary> = emptyList(),
    /** The store's categories, for the suggestions. */
    val categories: List<Category> = emptyList(),
    /** The store's item fields ("Serve em" …) with their values. */
    val attributes: List<Attribute> = emptyList(),
    /** The store's default assembly days, for "Padrão da loja"; null until the settings were read. */
    val defaultAssemblyDays: Int? = null,
    /** The store's default safety margin days, for "Padrão da loja"; null until the settings were read. */
    val defaultSafetyMarginDays: Int? = null,
    /** The choice of "Item a adicionar", or null at "Escolha o item". */
    val itemToAdd: String? = null,
    /** The role may change the catalog (`catalog:write`) and the editor could be set up for it. */
    val canWrite: Boolean = false,
    /** The "Fotos" section; its changes go through at once. */
    val photos: PhotosState = PhotosState(),
    /** "Salvar", "Arquivar" or "Restaurar" is under way. */
    val saving: Boolean = false,
    /** "Descartar as alterações?", while it asks. */
    val discard: DiscardAction? = null,
    /** The words of the store's niche, such as "Montagem". */
    val niche: NicheWords = NicheWords.Default,
) {
    /** True for "Novo kit": the kit was never saved. */
    val isNew: Boolean get() = kit == null

    /** The form can be edited: the role may write and the kit was read. */
    val editable: Boolean get() = canWrite && !loading && loadMessage == null

    /** The form takes edits now: the role may write, the kit was read, and nothing is being saved. */
    val canEdit: Boolean get() = editable && !saving

    /** Something was typed that is not saved: leaving asks first, and the totals say they are the last saved. */
    val dirty: Boolean get() = fields != initialFields

    /** The kit is archived: "Restaurar" shows in place of "Arquivar". */
    val archived: Boolean get() = kit?.archived == true

    /** The API's discount is negative: the kit costs more than its items. A warning, never a block. */
    val priceAboveItems: Boolean get() = kit?.discount?.cents?.let { it < 0 } == true

    /** The items "Item a adicionar" offers. */
    val addable: List<ItemSummary> get() = addableItems(catalogItems, fields.items)

    /** The problem marked next to [field], if any. */
    fun issue(field: KitField): FieldIssue? = issues[field]
}

/** KitFormEffect is something the editor asks its screen to do once. */
sealed interface KitFormEffect {
    /** The kit was archived or restored, or the user left: close the editor. */
    data object Close : KitFormEffect
}
