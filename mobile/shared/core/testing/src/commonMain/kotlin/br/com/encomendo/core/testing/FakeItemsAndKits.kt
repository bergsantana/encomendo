package br.com.encomendo.core.testing

import br.com.encomendo.core.domain.catalog.CostChange
import br.com.encomendo.core.domain.catalog.CustomizationOption
import br.com.encomendo.core.domain.catalog.Fetched
import br.com.encomendo.core.domain.catalog.Item
import br.com.encomendo.core.domain.catalog.ItemDraft
import br.com.encomendo.core.domain.catalog.ItemFilter
import br.com.encomendo.core.domain.catalog.ItemRepository
import br.com.encomendo.core.domain.catalog.ItemSummary
import br.com.encomendo.core.domain.catalog.Kit
import br.com.encomendo.core.domain.catalog.KitDays
import br.com.encomendo.core.domain.catalog.KitDraft
import br.com.encomendo.core.domain.catalog.KitFilter
import br.com.encomendo.core.domain.catalog.KitItem
import br.com.encomendo.core.domain.catalog.KitItemDraft
import br.com.encomendo.core.domain.catalog.KitRepository
import br.com.encomendo.core.domain.catalog.KitSummary
import br.com.encomendo.core.domain.catalog.OptionChoice
import br.com.encomendo.core.domain.catalog.OptionDraft
import br.com.encomendo.core.domain.catalog.OwnerField
import br.com.encomendo.core.domain.catalog.SupplierItem
import br.com.encomendo.core.domain.catalog.SupplierLinkDraft
import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.domain.failure.Reference
import br.com.encomendo.core.domain.money.Money
import br.com.encomendo.core.domain.paging.Page

private const val CURRENCY = "BRL"

/**
 * The fake's items, with the server's rules a screen meets: a stale `version` is a conflict, an item in a kit
 * that is not archived cannot be archived, archiving unpublishes, and a role that does not see costs
 * ([FakeCatalogState.seesCosts] false) is refused when it sends one.
 */
class FakeItemRepository(
    private val state: FakeCatalogState,
) : ItemRepository {
    private val builder = ItemBuilder(state)

    override suspend fun itemsPage(
        filter: ItemFilter,
        cursor: String?,
        limit: Int,
    ): Fetched<Page<ItemSummary>> {
        val rows =
            state.items.filter { item ->
                (filter.includeArchived || !item.archived) &&
                    item.matches(filter.query) &&
                    (filter.categoryId == null || item.categoryId == filter.categoryId)
            }
        return state.read(pageOf(rows.map { it.toSummary() }, cursor, limit))
    }

    override suspend fun item(id: String): Fetched<Item> =
        state.read(state.items.firstOrNull { it.id == id } ?: throw Failure.NotFound())

    override suspend fun createItem(draft: ItemDraft): Item {
        state.check()
        val created =
            with(builder) {
                refuseCosts(draft)
                draft.toItem(state.newId(), version = 1, previous = null)
            }
        state.items = state.items + created
        return created
    }

    override suspend fun replaceItem(
        id: String,
        version: Int,
        draft: ItemDraft,
    ): Item {
        with(builder) { refuseCosts(draft) }
        return change(id, version) { with(builder) { draft.toItem(id, it.version + 1, previous = it) } }
    }

    override suspend fun setItemPublished(
        id: String,
        published: Boolean,
        version: Int,
    ): Item = change(id, version) { it.copy(published = published && !it.archived, version = it.version + 1) }

    override suspend fun archiveItem(
        id: String,
        version: Int,
    ): Item {
        val holding = state.kits.filter { kit -> !kit.archived && kit.items.any { it.itemId == id } }
        if (holding.isNotEmpty()) throw Failure.ItemInKit(holding.map { Reference(it.id, it.name) })
        return change(id, version) { it.copy(archived = true, published = false, version = it.version + 1) }
    }

    override suspend fun restoreItem(
        id: String,
        version: Int,
    ): Item = change(id, version) { it.copy(archived = false, published = false, version = it.version + 1) }

    private fun change(
        id: String,
        version: Int,
        edit: (Item) -> Item,
    ): Item {
        state.check()
        val current = state.items.firstOrNull { it.id == id } ?: throw Failure.NotFound()
        if (current.version != version) throw Failure.VersionConflict()
        val changed = edit(current)
        state.items = state.items.map { if (it.id == id) changed else it }
        return changed
    }
}

/** Builds the items the fake saves from drafts, with the cost rules of ADR 0025. */
private class ItemBuilder(
    private val state: FakeCatalogState,
) {
    fun refuseCosts(draft: ItemDraft) {
        val sendsCost =
            draft.productionCost != CostChange.Keep ||
                listOfNotNull(draft.mainSupplier, draft.backupSupplier).any { it.cost != CostChange.Keep }
        if (sendsCost && !state.seesCosts) throw Failure.Forbidden()
    }

    fun ItemDraft.toItem(
        id: String,
        version: Int,
        previous: Item?,
    ): Item {
        val main = mainSupplier?.toSupplierItem(previous?.mainSupplier)
        return Item(
            id = id,
            name = name,
            description = description,
            categoryId = categoryId,
            sourcingMode = sourcingMode,
            price = Money(priceCents, CURRENCY),
            productionDays = productionDays,
            readyDays = main?.leadDays ?: productionDays ?: 0,
            mainSupplier = main,
            backupSupplier = backupSupplier?.toSupplierItem(previous?.backupSupplier),
            attributeValueIds = attributeValueIds,
            options = options.map { it.toOption(state) },
            photos = previous?.photos.orEmpty(),
            published = published,
            archived = previous?.archived ?: false,
            version = version,
            productionCost = costField(productionCost, previous?.productionCost),
            cost = main?.cost ?: OwnerField.Hidden,
            margin = if (state.seesCosts) OwnerField.Shown(null) else OwnerField.Hidden,
        )
    }

    private fun SupplierLinkDraft.toSupplierItem(previous: SupplierItem?): SupplierItem {
        val name = state.suppliers.firstOrNull { it.id == supplierId }?.name ?: throw Failure.NotFound()
        return SupplierItem(supplierId, name, leadDays, costField(cost, previous?.cost))
    }

    private fun costField(
        change: CostChange,
        previous: OwnerField<Money>?,
    ): OwnerField<Money> =
        when {
            !state.seesCosts -> OwnerField.Hidden
            change is CostChange.Set -> OwnerField.Shown(Money(change.cents, CURRENCY))
            change == CostChange.Keep -> previous ?: OwnerField.Shown(null)
            else -> OwnerField.Shown(null)
        }
}

private fun Item.matches(query: String?): Boolean = name.matches(query)

private fun String.matches(query: String?): Boolean = query == null || contains(query, ignoreCase = true)

private fun Item.toSummary(): ItemSummary =
    ItemSummary(
        id = id,
        name = name,
        categoryId = categoryId,
        sourcingMode = sourcingMode,
        price = price,
        readyDays = readyDays,
        mainSupplierName = mainSupplier?.supplierName,
        published = published,
        archived = archived,
        cover = photos.firstOrNull(),
        version = version,
        cost = cost,
        margin = margin,
    )

private fun OptionDraft.toOption(state: FakeCatalogState): CustomizationOption =
    CustomizationOption(
        id = id ?: state.newId(),
        name = name,
        required = required,
        choices =
            choices.map {
                OptionChoice(it.id ?: state.newId(), it.label, Money(it.extraPriceCents, CURRENCY), it.extraDays)
            },
    )

/** Pages [rows] the way the API does: [cursor] is where the page starts, and the last page has none. */
internal fun <T> pageOf(
    rows: List<T>,
    cursor: String?,
    limit: Int,
): Page<T> {
    val start = cursor?.toIntOrNull() ?: 0
    val end = (start + limit).coerceAtMost(rows.size)
    return Page(rows.subList(start.coerceAtMost(end), end), if (end < rows.size) end.toString() else null)
}

/**
 * The fake's kits, with the server's rules: an archived item (or alternative) refuses a save or restore, a
 * stale `version` is a conflict. Totals and days are plain sums, enough for a screen test; seed [Kit]s in
 * [FakeCatalogState.kits] when a test needs the server's exact numbers.
 */
class FakeKitRepository(
    private val state: FakeCatalogState,
) : KitRepository {
    private val builder = KitBuilder(state)

    override suspend fun kitsPage(
        filter: KitFilter,
        cursor: String?,
        limit: Int,
    ): Fetched<Page<KitSummary>> {
        val rows =
            state.kits.filter { kit ->
                (filter.includeArchived || !kit.archived) &&
                    kit.name.matches(filter.query)
            }
        return state.read(pageOf(rows.map { it.toSummary() }, cursor, limit))
    }

    override suspend fun kit(id: String): Fetched<Kit> =
        state.read(state.kits.firstOrNull { it.id == id } ?: throw Failure.NotFound())

    override suspend fun createKit(draft: KitDraft): Kit {
        state.check()
        val created = with(builder) { draft.toKit(state.newId(), version = 1, previous = null) }
        state.kits = state.kits + created
        return created
    }

    override suspend fun replaceKit(
        id: String,
        version: Int,
        draft: KitDraft,
    ): Kit = change(id, version) { with(builder) { draft.toKit(id, it.version + 1, previous = it) } }

    override suspend fun setKitPublished(
        id: String,
        published: Boolean,
        version: Int,
    ): Kit = change(id, version) { it.copy(published = published && !it.archived, version = it.version + 1) }

    override suspend fun archiveKit(
        id: String,
        version: Int,
    ): Kit = change(id, version) { it.copy(archived = true, published = false, version = it.version + 1) }

    override suspend fun restoreKit(
        id: String,
        version: Int,
    ): Kit =
        change(id, version) {
            builder.refuseArchived(it.items.map(KitItem::itemId))
            it.copy(archived = false, published = false, version = it.version + 1)
        }

    private fun change(
        id: String,
        version: Int,
        edit: (Kit) -> Kit,
    ): Kit {
        state.check()
        val current = state.kits.firstOrNull { it.id == id } ?: throw Failure.NotFound()
        if (current.version != version) throw Failure.VersionConflict()
        val changed = edit(current)
        state.kits = state.kits.map { if (it.id == id) changed else it }
        return changed
    }
}

/** Builds the kits the fake saves from drafts, refusing archived items as the server does. */
private class KitBuilder(
    private val state: FakeCatalogState,
) {
    private fun itemOf(id: String): Item = state.items.firstOrNull { it.id == id } ?: throw Failure.NotFound()

    private fun Item.toKitItem(draft: KitItemDraft): KitItem =
        KitItem(id, name, draft.quantity, draft.swappable, draft.alternativeItemIds, price, readyDays, archived)

    fun refuseArchived(itemIds: List<String>) {
        val archived = state.items.filter { it.archived && it.id in itemIds }
        if (archived.isNotEmpty()) throw Failure.ItemArchived(archived.map { Reference(it.id, it.name) })
    }

    fun KitDraft.toKit(
        id: String,
        version: Int,
        previous: Kit?,
    ): Kit {
        refuseArchived(items.flatMap { listOf(it.itemId) + it.alternativeItemIds })
        val rows = items.map { draft -> draft to itemOf(draft.itemId) }
        val singleTotal = rows.sumOf { (draft, item) -> item.price.cents * draft.quantity }
        val slowest = rows.maxOfOrNull { (_, item) -> item.readyDays } ?: 0
        val total = slowest + (assemblyDays ?: 0) + (safetyMarginDays ?: 0)
        return Kit(
            id = id,
            name = name,
            description = description,
            price = Money(priceCents, CURRENCY),
            singleItemsTotal = Money(singleTotal, CURRENCY),
            discount = Money(singleTotal - priceCents, CURRENCY),
            swapDiscount = swapDiscount,
            assemblyDays = assemblyDays,
            safetyMarginDays = safetyMarginDays,
            days = KitDays(slowest, assembly = assemblyDays ?: 0, safetyMargin = safetyMarginDays ?: 0, total = total),
            items = rows.map { (draft, item) -> item.toKitItem(draft) },
            attributeValueIds = attributeValueIds,
            photos = previous?.photos.orEmpty(),
            published = published,
            archived = previous?.archived ?: false,
            version = version,
            cost = if (state.seesCosts) OwnerField.Shown(null) else OwnerField.Hidden,
            margin = if (state.seesCosts) OwnerField.Shown(null) else OwnerField.Hidden,
        )
    }
}

private fun Kit.toSummary(): KitSummary =
    KitSummary(
        id = id,
        name = name,
        itemCount = items.size,
        price = price,
        singleItemsTotal = singleItemsTotal,
        totalDays = days.total,
        published = published,
        archived = archived,
        cover = photos.firstOrNull(),
        version = version,
        cost = cost,
        margin = margin,
    )
