package br.com.encomendo.core.network.catalog

import br.com.encomendo.core.network.API_URL
import br.com.encomendo.core.network.ApiClientConfig
import br.com.encomendo.core.network.InMemoryTokenStorage
import br.com.encomendo.core.network.SessionTurns
import br.com.encomendo.core.network.TestTokens
import br.com.encomendo.core.network.TokenRefresher
import br.com.encomendo.core.network.createApiClient
import io.ktor.client.HttpClient
import io.ktor.client.engine.mock.MockEngine
import io.ktor.client.engine.mock.MockRequestHandler
import io.ktor.client.engine.mock.toByteArray
import io.ktor.client.request.HttpRequestData

/** An API client over [handler] that signs in as the store's owner, as the repositories' tests do. */
internal fun catalogClient(handler: MockRequestHandler): HttpClient =
    createApiClient(
        engine = MockEngine(handler),
        config = ApiClientConfig(baseUrl = API_URL),
        tokens = InMemoryTokenStorage(TestTokens(accessToken = "access-alta-rpm", refreshToken = "refresh-old")),
        refresher = TokenRefresher<TestTokens> { _, _ -> error("no refresh expected") },
        turns = SessionTurns(),
    )

internal fun catalogApi(handler: MockRequestHandler): CatalogApis = CatalogApis(catalogClient(handler))

internal suspend fun HttpRequestData.bodyText(): String = body.toByteArray().decodeToString()

internal const val ITEM_ID = "0190f3c4-0000-7000-8000-0000000000a1"
internal const val KIT_ID = "0190f3c4-0000-7000-8000-0000000000b1"
internal const val SUPPLIER_ID = "0190f3c4-0000-7000-8000-0000000000c1"
internal const val PHOTO_ID = "0190f3c4-0000-7000-8000-0000000000d1"

/** The `cover` photo of a list row. */
internal const val PHOTO_JSON =
    """{"id": "$PHOTO_ID", "url": "https://files.test/p1?sig=x", "expiresAt": "2026-11-27T13:00:00Z", "position": 0}"""

/**
 * An ItemSummary as docs/api/openapi.yaml describes it, with [extra] after the common fields: the owner-only
 * `cost` and `margin`, or nothing for another role.
 */
internal fun itemSummaryJson(extra: String = ""): String =
    """
    {"id": "$ITEM_ID", "name": "Banco de couro Scrambler", "categoryId": null, "sourcingMode": "to_order",
     "price": {"cents": 89000, "currency": "BRL"}, "readyDays": 15, "mainSupplierName": "Couros Paulista",
     "published": true, "archived": false, "cover": $PHOTO_JSON, "version": 3 $extra}
    """.trimIndent()

/** An Item with the owner-only fields in [extra] and the main supplier's [supplierExtra]. */
internal fun itemJson(
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

/** A Kit with the owner-only fields in [extra]. */
internal fun kitJson(extra: String = ""): String =
    """
    {"id": "$KIT_ID", "name": "Kit Scrambler CB 500X", "description": "",
     "price": {"cents": 250000, "currency": "BRL"}, "singleItemsTotal": {"cents": 280000, "currency": "BRL"},
     "discount": {"cents": 30000, "currency": "BRL"}, "swapDiscount": "keep", "assemblyDays": null,
     "safetyMarginDays": 2, "days": {"slowestItem": 15, "assembly": 3, "safetyMargin": 2, "total": 20},
     "items": [{"itemId": "$ITEM_ID", "itemName": "Banco", "quantity": 1, "swappable": true,
                "alternativeItemIds": ["$SUPPLIER_ID"], "unitPrice": {"cents": 89000, "currency": "BRL"},
                "readyDays": 15, "archived": false}],
     "attributeValueIds": [], "photos": [], "published": false, "archived": false, "version": 1 $extra}
    """.trimIndent()

internal const val SUPPLIER_JSON =
    """{"id": "$SUPPLIER_ID", "name": "Couros Paulista", "whatsappPhone": "5511987654321", "version": 2}"""
