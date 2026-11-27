package br.com.encomendo.feature.catalogo

import br.com.encomendo.core.domain.catalog.Attribute
import br.com.encomendo.core.domain.catalog.Category
import br.com.encomendo.core.domain.catalog.Item
import br.com.encomendo.core.domain.catalog.Photo
import br.com.encomendo.core.domain.catalog.SourcingMode
import br.com.encomendo.core.domain.supplier.Supplier

/**
 * ItemFormUiState is the item editor ("Novo item" or "Editar item"): what was typed, the problems marked next to
 * the fields, the choices to pick from, and what the role may do. A role without `catalog:write` gets the same
 * state with [canWrite] false, and the screen then shows the item read-only.
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
    /** The role may change the catalog (`catalog:write`). */
    val canWrite: Boolean = false,
    /** The API sends costs to this role, so the cost fields show. */
    val showsCosts: Boolean = false,
    /** The item is archived: "Restaurar" shows in place of "Arquivar". */
    val archived: Boolean = false,
    /** The version to save against; null for a new item. */
    val version: Int? = null,
    /** The item's photos, the first being the cover. */
    val photos: List<Photo> = emptyList(),
    /** A photo is being uploaded, ordered or removed. */
    val photoBusy: Boolean = false,
    /** Why the last photo change failed, under "Fotos". */
    val photoMessage: CatalogMessage? = null,
    /** "Salvar", "Arquivar" or "Restaurar" is under way. */
    val saving: Boolean = false,
    /** "Novo fornecedor", while open. */
    val newSupplier: NewSupplierForm? = null,
) {
    /** True for "Novo item". */
    val isNew: Boolean get() = itemId == null

    /** The form can be edited: the role may write and the item was read. */
    val editable: Boolean get() = canWrite && !loading && loadMessage == null

    /** A photo change may start: a saved item the role may change, with no other photo change under way. */
    val canChangePhotos: Boolean get() = editable && !isNew && !photoBusy

    /** The name of the supplier [id], whoever lists it. */
    fun supplierName(id: String?): String? = suppliers.firstOrNull { it.id == id }?.name

    /** The problem marked next to [field], if any. */
    fun issue(field: ItemField): FieldIssue? = issues[field]
}

/** ItemFormEffect is something the form asks its screen to do once. */
sealed interface ItemFormEffect {
    /** The item was saved, archived or restored: leave the form. */
    data object Close : ItemFormEffect
}

/** PhotoSource says where "Adicionar fotos" gets a photo. */
enum class PhotoSource {
    /** The phone's gallery. */
    Gallery,

    /** The camera. */
    Camera,
}
