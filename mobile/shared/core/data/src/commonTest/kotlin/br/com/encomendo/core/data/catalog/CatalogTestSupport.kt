package br.com.encomendo.core.data.catalog

import br.com.encomendo.core.database.CatalogCacheDao
import br.com.encomendo.core.database.CatalogDetailEntity
import br.com.encomendo.core.database.CatalogPageEntity
import br.com.encomendo.core.domain.catalog.AttributeDraft
import br.com.encomendo.core.domain.catalog.AttributeValueDraft
import br.com.encomendo.core.domain.catalog.CostChange
import br.com.encomendo.core.domain.catalog.ItemDraft
import br.com.encomendo.core.domain.catalog.KitDraft
import br.com.encomendo.core.domain.catalog.KitItemDraft
import br.com.encomendo.core.domain.catalog.SourcingMode
import br.com.encomendo.core.domain.catalog.SupplierLinkDraft
import br.com.encomendo.core.domain.catalog.SwapDiscount
import kotlin.time.Clock
import kotlin.time.Instant

const val ITEM_ID = "0190f3c4-0000-7000-8000-0000000000a1"
const val KIT_ID = "0190f3c4-0000-7000-8000-0000000000b1"
const val SUPPLIER_ID = "0190f3c4-0000-7000-8000-0000000000c1"
const val PHOTO_ID = "0190f3c4-0000-7000-8000-0000000000d1"
const val PHOTO_KEY = "t/0190f3c4-0000-7000-8000-000000000001/items/$ITEM_ID/$PHOTO_ID"

const val PHOTO_JSON =
    """{"id": "$PHOTO_ID", "url": "https://files.test/p1?sig=x", "expiresAt": "2026-11-27T13:00:00Z", "position": 0}"""

/** An ItemSummary; [extra] holds the owner-only `cost` and `margin`, or nothing for another role. */
fun itemSummaryJson(extra: String = ""): String =
    """
    {"id": "$ITEM_ID", "name": "Banco de couro Scrambler", "categoryId": null, "sourcingMode": "to_order",
     "price": {"cents": 89000, "currency": "BRL"}, "readyDays": 15, "mainSupplierName": "Couros Paulista",
     "published": true, "archived": false, "cover": $PHOTO_JSON, "version": 3 $extra}
    """.trimIndent()

fun itemPageJson(extra: String = ""): String = """{"items": [${itemSummaryJson(extra)}], "nextCursor": "c2"}"""

/** An Item; [extra] holds the owner-only fields and [supplierExtra] the main supplier's cost. */
fun itemJson(
    extra: String = "",
    supplierExtra: String = "",
): String =
    """
    {"id": "$ITEM_ID", "name": "Banco de couro Scrambler", "description": "", "categoryId": null,
     "sourcingMode": "to_order", "price": {"cents": 89000, "currency": "BRL"}, "productionDays": null,
     "readyDays": 15,
     "mainSupplier": {"supplierId": "$SUPPLIER_ID", "supplierName": "Couros Paulista", "leadDays": 15 $supplierExtra},
     "backupSupplier": null, "attributeValueIds": [], "options": [], "photos": [$PHOTO_JSON],
     "published": true, "archived": false, "version": 3 $extra}
    """.trimIndent()

fun kitJson(extra: String = ""): String =
    """
    {"id": "$KIT_ID", "name": "Kit Scrambler CB 500X", "description": "",
     "price": {"cents": 250000, "currency": "BRL"}, "singleItemsTotal": {"cents": 280000, "currency": "BRL"},
     "discount": {"cents": 30000, "currency": "BRL"}, "swapDiscount": "keep", "assemblyDays": null,
     "safetyMarginDays": 2, "days": {"slowestItem": 15, "assembly": 3, "safetyMargin": 2, "total": 20},
     "items": [{"itemId": "$ITEM_ID", "itemName": "Banco", "quantity": 1, "swappable": false,
                "alternativeItemIds": [], "unitPrice": {"cents": 89000, "currency": "BRL"},
                "readyDays": 15, "archived": false}],
     "attributeValueIds": [], "photos": [], "published": false, "archived": false, "version": 1 $extra}
    """.trimIndent()

const val SUPPLIER_JSON =
    """{"id": "$SUPPLIER_ID", "name": "Couros Paulista", "whatsappPhone": "5511987654321", "version": 2}"""

const val CATEGORY_JSON = """{"id": "$ITEM_ID", "name": "Bancos", "position": 0}"""

const val ATTRIBUTE_JSON =
    """{"id": "$ITEM_ID", "name": "Serve em", "position": 0,
        "values": [{"id": "$KIT_ID", "label": "CB 500X", "usageCount": 4}]}"""

fun itemDraft(
    mainCost: CostChange = CostChange.Keep,
    productionCost: CostChange = CostChange.Keep,
) = ItemDraft(
    name = "Banco de couro Scrambler",
    description = "",
    categoryId = null,
    sourcingMode = SourcingMode.ToOrder,
    priceCents = 89000,
    productionDays = null,
    productionCost = productionCost,
    mainSupplier = SupplierLinkDraft(SUPPLIER_ID, leadDays = 15, cost = mainCost),
    backupSupplier = null,
    attributeValueIds = emptyList(),
    options = emptyList(),
    published = false,
)

fun kitDraft() =
    KitDraft(
        name = "Kit Scrambler CB 500X",
        description = "",
        priceCents = 250000,
        swapDiscount = SwapDiscount.Keep,
        assemblyDays = null,
        safetyMarginDays = 2,
        items = listOf(KitItemDraft(ITEM_ID, quantity = 1, swappable = false, alternativeItemIds = emptyList())),
        attributeValueIds = emptyList(),
        published = false,
    )

fun attributeDraft() = AttributeDraft("Serve em", listOf(AttributeValueDraft(id = null, label = "CB 500X")))

/** A clock that says [epochMillis] until a test moves it. */
class FixedClock(
    var epochMillis: Long = 1_700_000_000_000,
) : Clock {
    override fun now(): Instant = Instant.fromEpochMilliseconds(epochMillis)
}

/**
 * The offline cache's DAO in memory, so the repository tests need no database. It follows the contract the
 * real DAO has (one row per key; a first page replaces its list), which the Room tests of core:database check.
 */
class InMemoryCatalogCacheDao : CatalogCacheDao() {
    /** The pages saved, by (store, costs, kind, query, cursor). */
    val pages = mutableMapOf<List<Any>, CatalogPageEntity>()

    /** The details saved, by (store, costs, kind, id). */
    val details = mutableMapOf<List<Any>, CatalogDetailEntity>()

    override suspend fun page(
        tenantId: String,
        costs: Boolean,
        kind: String,
        query: String,
        cursor: String,
    ): CatalogPageEntity? = pages[listOf(tenantId, costs, kind, query, cursor)]

    override suspend fun detail(
        tenantId: String,
        costs: Boolean,
        kind: String,
        id: String,
    ): CatalogDetailEntity? = details[listOf(tenantId, costs, kind, id)]

    override suspend fun upsertPage(page: CatalogPageEntity) {
        pages[listOf(page.tenantId, page.costs, page.kind, page.query, page.cursor)] = page
    }

    override suspend fun upsertDetail(detail: CatalogDetailEntity) {
        details[listOf(detail.tenantId, detail.costs, detail.kind, detail.id)] = detail
    }

    override suspend fun deletePagesOfList(
        tenantId: String,
        costs: Boolean,
        kind: String,
        query: String,
    ) {
        pages.keys.removeAll { it.take(4) == listOf(tenantId, costs, kind, query) }
    }

    override suspend fun trimLists(
        tenantId: String,
        costs: Boolean,
        kind: String,
        keep: Int,
    ) = Unit

    override suspend fun trimDetails(
        tenantId: String,
        costs: Boolean,
        kind: String,
        keep: Int,
    ) = Unit

    override suspend fun dropOtherViewPages(
        tenantId: String,
        costs: Boolean,
    ) {
        pages.keys.removeAll { it[0] == tenantId && it[1] != costs }
    }

    override suspend fun dropOtherViewDetails(
        tenantId: String,
        costs: Boolean,
    ) {
        details.keys.removeAll { it[0] == tenantId && it[1] != costs }
    }
}
