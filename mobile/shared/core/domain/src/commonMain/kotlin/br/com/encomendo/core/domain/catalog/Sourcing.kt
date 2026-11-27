package br.com.encomendo.core.domain.catalog

import br.com.encomendo.core.domain.money.Money

/**
 * SourcingMode says where an item comes from: Sob encomenda, Estoque or
 * Produção própria (`catalog.SourcingMode`). Rules hang on it; labels come
 * from the store's words, never from [key].
 */
enum class SourcingMode(
    /** The value the API uses. */
    val key: String,
) {
    /** Bought from a supplier once sold. */
    ToOrder("to_order"),

    /** Kept on the shelf. */
    Stock("stock"),

    /** Made by the store itself; it has no supplier. */
    InHouse("in_house"),
    ;

    /** Holds the lookup from the API's key. */
    companion object {
        /** Returns the mode the API calls [key], or null for a value this app does not know. */
        fun fromKey(key: String): SourcingMode? = entries.firstOrNull { it.key == key }
    }
}

/** Margin is the price minus the cost, computed by the server (`Margin` in docs/api/openapi.yaml). */
data class Margin(
    /** Price minus cost; negative when the cost is above the price. */
    val cents: Long,
    /** The margin over the price in hundredths of a percent: 3000 is 30%. */
    val basisPoints: Int,
)

/** SupplierItem is a supplier of an item with its lead time and, for the owner, its cost. */
data class SupplierItem(
    /** The supplier's id (a UUID). */
    val supplierId: String,
    /** The supplier's name. */
    val supplierName: String,
    /** The days this supplier needs to deliver. */
    val leadDays: Int,
    /** The cost of this item from this supplier; [OwnerField.Hidden] for every role but the owner. */
    val cost: OwnerField<Money>,
)

/** SupplierLinkDraft is a supplier of an item as the user edited it. */
data class SupplierLinkDraft(
    /** The supplier's id (a UUID). */
    val supplierId: String,
    /** The days this supplier needs to deliver. */
    val leadDays: Int,
    /** What to do with the cost; always [CostChange.Keep] unless the owner touched it. */
    val cost: CostChange = CostChange.Keep,
)
