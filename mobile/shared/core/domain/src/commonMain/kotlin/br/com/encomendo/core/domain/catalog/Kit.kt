package br.com.encomendo.core.domain.catalog

import br.com.encomendo.core.domain.money.Money

/** SwapDiscount says what a client's swap does to a kit's price (ADR 0023). */
enum class SwapDiscount(
    /** The value the API uses. */
    val key: String,
) {
    /** Kit price minus the removed item plus the new one, both at single-item price. */
    Keep("keep"),

    /** The sum of the resulting items' single-item prices. */
    Lose("lose"),
    ;

    /** Holds the lookup from the API's key. */
    companion object {
        /** Returns the mode the API calls [key], or null for a value this app does not know. */
        fun fromKey(key: String): SwapDiscount? = entries.firstOrNull { it.key == key }
    }
}

/** KitSummary is a row of the Kits list (`KitSummary` in docs/api/openapi.yaml). */
data class KitSummary(
    /** The kit's id (a UUID). */
    val id: String,
    /** The kit's name. */
    val name: String,
    /** How many different items the kit holds. */
    val itemCount: Int,
    /** The kit's price. */
    val price: Money,
    /** The sum of the single items, quantities included. */
    val singleItemsTotal: Money,
    /** "Prazo calculado" in days, from the API. */
    val totalDays: Int,
    /** Shown on the store page ("No portal"). */
    val published: Boolean,
    /** Archived kits are hidden from the store page and the pickers. */
    val archived: Boolean,
    /** The first photo, or null when there is none. */
    val cover: Photo?,
    /** The version to send back when saving. */
    val version: Int,
    /** The sum of the items' costs; hidden for every role but the owner, null when one has none. */
    val cost: OwnerField<Money>,
    /** Kit price minus cost; hidden for every role but the owner, null without a cost. */
    val margin: OwnerField<Margin>,
)

/** Kit is one kit with its items, totals and days (`Kit` in docs/api/openapi.yaml). */
data class Kit(
    /** The kit's id (a UUID). */
    val id: String,
    /** The kit's name. */
    val name: String,
    /** The description; empty when there is none. */
    val description: String,
    /** The kit's price. */
    val price: Money,
    /** The sum of the single items, quantities included. */
    val singleItemsTotal: Money,
    /** Desconto do kit; negative when the kit costs more than its items. */
    val discount: Money,
    /** What a swap does to the price. */
    val swapDiscount: SwapDiscount,
    /** The kit's own assembly days; null uses the store's default. */
    val assemblyDays: Int?,
    /** The kit's own safety margin days; null uses the store's default. */
    val safetyMarginDays: Int?,
    /** "Prazo calculado", from the API. */
    val days: KitDays,
    /** The items, at least one. */
    val items: List<KitItem>,
    /** The ids of the attribute values the kit picked. */
    val attributeValueIds: List<String>,
    /** Up to 8 photos; the first is the cover. */
    val photos: List<Photo>,
    /** Shown on the store page ("No portal"). */
    val published: Boolean,
    /** Archived kits are hidden from the store page and the pickers. */
    val archived: Boolean,
    /** The version to send back when saving. */
    val version: Int,
    /** The sum of the items' costs times their quantities; hidden for every role but the owner. */
    val cost: OwnerField<Money>,
    /** Kit price minus cost; hidden for every role but the owner, null without a cost. */
    val margin: OwnerField<Margin>,
)

/** KitDays is "Prazo calculado": slowest item plus assembly plus safety margin, computed by the server. */
data class KitDays(
    /** The slowest item's ready days. */
    val slowestItem: Int,
    /** The assembly days in use: the kit's own, or the store's default. */
    val assembly: Int,
    /** The safety margin days in use: the kit's own, or the store's default. */
    val safetyMargin: Int,
    /** The total. */
    val total: Int,
)

/** KitItem is one item of a kit with its quantity and the items a client may swap it for. */
data class KitItem(
    /** The item's id (a UUID). */
    val itemId: String,
    /** The item's name. */
    val itemName: String,
    /** How many of it the kit holds. */
    val quantity: Int,
    /** Cliente pode trocar. */
    val swappable: Boolean,
    /** Pode trocar por; empty unless [swappable]. */
    val alternativeItemIds: List<String>,
    /** The item's single-item price. */
    val unitPrice: Money,
    /** The item's ready days. */
    val readyDays: Int,
    /** True when the item was archived after the kit was saved; the kit cannot be saved until it is replaced. */
    val archived: Boolean,
)

/** KitDraft is a kit as the user edited it, ready to create or replace (`KitInput`). */
data class KitDraft(
    /** The kit's name. */
    val name: String,
    /** The description; empty when there is none. */
    val description: String,
    /** The kit's price in cents of the store's currency. */
    val priceCents: Long,
    /** What a swap does to the price. */
    val swapDiscount: SwapDiscount,
    /** The kit's own assembly days; null uses the store's default. */
    val assemblyDays: Int?,
    /** The kit's own safety margin days; null uses the store's default. */
    val safetyMarginDays: Int?,
    /** The items, at least one. */
    val items: List<KitItemDraft>,
    /** The attribute values the kit picks. */
    val attributeValueIds: List<String>,
    /** Whether the kit is shown on the store page. */
    val published: Boolean,
)

/** KitItemDraft is one item of a [KitDraft]. */
data class KitItemDraft(
    /** The item's id (a UUID). */
    val itemId: String,
    /** How many of it the kit holds, 1 to 99. */
    val quantity: Int,
    /** Whether the client may swap it. */
    val swappable: Boolean,
    /** What it may be swapped for; at least one when [swappable], empty otherwise. */
    val alternativeItemIds: List<String>,
)
