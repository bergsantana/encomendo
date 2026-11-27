package br.com.encomendo.feature.catalogo

import br.com.encomendo.core.domain.catalog.Attribute
import br.com.encomendo.core.domain.catalog.Category
import br.com.encomendo.core.domain.catalog.Item
import br.com.encomendo.core.domain.catalog.SourcingMode
import br.com.encomendo.core.domain.supplier.Supplier

/**
 * ItemFormUiState is the item editor ("Novo item" or "Editar item"): what was typed, the problems marked next to
 * the fields, the choices to pick from, and what the role may do. A role without `catalog:write`, or an editor whose
 * store settings could not be read offline, gets the same state with [canWrite] false, and the screen then shows the
 * item read-only.
 */
data class ItemFormUiState(
    /** The item being edited; null for "Novo item". */
    val itemId: String? = null,
    /** The item and the choices are being read. */
    val loading: Boolean = true,
    /** Why the item could not be read: the message and "Tentar de novo" show instead of the form. */
    val loadMessage: CatalogMessage? = null,
    /** When the item shown was saved on the device, in epoch milliseconds, if the server did not answer. */
    val offlineSince: Long? = null,
    /** The item as read, for the read-only view; null for a new item. */
    val item: Item? = null,
    /** What was typed. */
    val fields: ItemFields = ItemFields(),
    /** What the form started with, to tell whether anything was typed since. */
    val initialFields: ItemFields = ItemFields(),
    /** The problems marked next to their fields. */
    val issues: Map<ItemField, FieldIssue> = emptyMap(),
    /** Why the last change failed, above the form. */
    val message: CatalogMessage? = null,
    /** The store's categories. */
    val categories: List<Category> = emptyList(),
    /** The store's item fields ("Serve em" …) with their values. */
    val attributes: List<Attribute> = emptyList(),
    /** The store's suppliers, for "Fornecedor principal" and "Fornecedor reserva". */
    val suppliers: List<Supplier> = emptyList(),
    /** The "Origem" choices the store allows, plus the item's own when it has one that is now off. */
    val modes: List<SourcingMode> = listOf(SourcingMode.ToOrder),
    /** The role may change the catalog (`catalog:write`) and the form could be set up for it. */
    val canWrite: Boolean = false,
    /** The API sends costs to this role, so the cost fields show. */
    val showsCosts: Boolean = false,
    /** The item is archived: "Restaurar" shows in place of "Arquivar". */
    val archived: Boolean = false,
    /** The version to save against; null for a new item. */
    val version: Int? = null,
    /** The "Fotos" section; its changes go through at once. */
    val photos: PhotosState = PhotosState(),
    /** "Salvar", "Arquivar" or "Restaurar" is under way. */
    val saving: Boolean = false,
    /** "Novo fornecedor", while open. */
    val newSupplier: NewSupplierForm? = null,
    /** "Descartar as alterações?", while it asks. */
    val discard: DiscardAction? = null,
) {
    /** True for "Novo item". */
    val isNew: Boolean get() = itemId == null

    /** The form can be edited: the role may write and the item was read. */
    val editable: Boolean get() = canWrite && !loading && loadMessage == null

    /** Something was typed that is not saved: leaving asks first. */
    val dirty: Boolean get() = fields != initialFields

    /** The name of the supplier [id], whoever lists it. */
    fun supplierName(id: String?): String? = suppliers.firstOrNull { it.id == id }?.name

    /** The problem marked next to [field], if any. */
    fun issue(field: ItemField): FieldIssue? = issues[field]
}

/** DiscardAction is what the form was about to do when it asked "Descartar as alterações?". */
enum class DiscardAction {
    /** Leave the form ("Cancelar", or Back). */
    Close,

    /** "Arquivar" the item. */
    Archive,

    /** "Restaurar" the item. */
    Restore,
}

/** ItemFormEffect is something the form asks its screen to do once. */
sealed interface ItemFormEffect {
    /** The item was saved, archived or restored, or the user left: close the form. */
    data object Close : ItemFormEffect
}
