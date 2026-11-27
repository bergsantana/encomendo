package br.com.encomendo.core.domain.catalog

import br.com.encomendo.core.domain.money.Money

/** ItemSummary is a row of the Itens list (`ItemSummary` in docs/api/openapi.yaml). */
data class ItemSummary(
    /** The item's id (a UUID). */
    val id: String,
    /** The item's name. */
    val name: String,
    /** The item's category id, or null for "Sem categoria". */
    val categoryId: String?,
    /** Where the item comes from. */
    val sourcingMode: SourcingMode,
    /** The single-item price, from the API. */
    val price: Money,
    /** Days the item needs before it is ready, from the API. */
    val readyDays: Int,
    /** The main supplier's name; null for in-house items. */
    val mainSupplierName: String?,
    /** Shown on the store page ("No portal"). */
    val published: Boolean,
    /** Archived items are hidden from the store page and the pickers. */
    val archived: Boolean,
    /** The first photo, or null when there is none. */
    val cover: Photo?,
    /** The version to send back when saving. */
    val version: Int,
    /** The cost the margin uses; hidden for every role but the owner. */
    val cost: OwnerField<Money>,
    /** Price minus cost; hidden for every role but the owner, and null without a cost. */
    val margin: OwnerField<Margin>,
)

/** Item is one catalog item with its suppliers, fields, options and photos (`Item` in docs/api/openapi.yaml). */
data class Item(
    /** The item's id (a UUID). */
    val id: String,
    /** The item's name. */
    val name: String,
    /** The description; empty when there is none. */
    val description: String,
    /** The item's category id, or null for "Sem categoria". */
    val categoryId: String?,
    /** Where the item comes from. */
    val sourcingMode: SourcingMode,
    /** The single-item price, from the API. */
    val price: Money,
    /** The days an in-house item takes to make; null for the other modes. */
    val productionDays: Int?,
    /** Days the item needs before it is ready, from the API. */
    val readyDays: Int,
    /** The main supplier; null for in-house items. */
    val mainSupplier: SupplierItem?,
    /** Fornecedor reserva; null when there is none. */
    val backupSupplier: SupplierItem?,
    /** The ids of the attribute values the item picked. */
    val attributeValueIds: List<String>,
    /** The choices a client makes when ordering. */
    val options: List<CustomizationOption>,
    /** Up to 8 photos; the first is the cover. */
    val photos: List<Photo>,
    /** Shown on the store page ("No portal"). */
    val published: Boolean,
    /** Archived items are hidden from the store page and the pickers. */
    val archived: Boolean,
    /** The version to send back when saving. */
    val version: Int,
    /** The production cost; shown only to the owner and only for in-house items. */
    val productionCost: OwnerField<Money>,
    /** The cost the margin uses; hidden for every role but the owner. */
    val cost: OwnerField<Money>,
    /** Price minus cost; hidden for every role but the owner, and null without a cost. */
    val margin: OwnerField<Margin>,
)

/** CustomizationOption is a choice the client makes when ordering: exactly one [choices] entry when [required]. */
data class CustomizationOption(
    /** The option's id (a UUID). */
    val id: String,
    /** The option's name, such as "Cor da costura". */
    val name: String,
    /** Whether the client must pick a choice. */
    val required: Boolean,
    /** The choices, at least one. */
    val choices: List<OptionChoice>,
)

/** OptionChoice is one choice of a [CustomizationOption], with what it adds to the price and the days. */
data class OptionChoice(
    /** The choice's id (a UUID). */
    val id: String,
    /** The label, such as "Vermelha". */
    val label: String,
    /** What the choice adds to the price. */
    val extraPrice: Money,
    /** The days the choice adds to the ready date. */
    val extraDays: Int,
)

/** ItemDraft is an item as the user edited it, ready to create or replace (`ItemInput`). */
data class ItemDraft(
    /** The item's name. */
    val name: String,
    /** The description; empty when there is none. */
    val description: String,
    /** The category id, or null for "Sem categoria". */
    val categoryId: String?,
    /** Where the item comes from. */
    val sourcingMode: SourcingMode,
    /** The single-item price in cents of the store's currency. */
    val priceCents: Long,
    /** Required for in-house items; null otherwise. */
    val productionDays: Int?,
    /** What to do with the production cost; [CostChange.Keep] unless the owner touched it. */
    val productionCost: CostChange,
    /** Required for `to_order` and `stock`; null for in-house items. */
    val mainSupplier: SupplierLinkDraft?,
    /** An optional supplier other than the main one. */
    val backupSupplier: SupplierLinkDraft?,
    /** The attribute values the item picks. */
    val attributeValueIds: List<String>,
    /** The options clients choose from. */
    val options: List<OptionDraft>,
    /** Whether the item is shown on the store page. */
    val published: Boolean,
)

/** OptionDraft is a customization option as the user edited it. */
data class OptionDraft(
    /** The id of the existing option this one keeps; null for a new one. */
    val id: String?,
    /** The option's name. */
    val name: String,
    /** Whether the client must pick a choice. */
    val required: Boolean,
    /** The choices, at least one. */
    val choices: List<OptionChoiceDraft>,
)

/** OptionChoiceDraft is one choice of an [OptionDraft]. */
data class OptionChoiceDraft(
    /** The id of the existing choice this one keeps; null for a new one. */
    val id: String?,
    /** The label. */
    val label: String,
    /** What the choice adds to the price, in cents. */
    val extraPriceCents: Long,
    /** The days the choice adds. */
    val extraDays: Int,
)
