package br.com.encomendo.core.network.catalog

import br.com.encomendo.core.network.ApiPayloadJson
import br.com.encomendo.core.network.respondJson
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.encodeToString
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse

class OwnerFieldDtoTest {
    @Test
    fun aFieldTheApiLeftOutIsAbsentForAnotherRole() =
        runTest {
            // An admin's answer: no cost, no margin, no supplier cost. Hidden, not zero and not null.
            val api = catalogApi { respondJson(itemJson()) }

            val item = api.items.get(ITEM_ID)

            assertEquals(OwnerFieldDto.Absent, item.cost)
            assertEquals(OwnerFieldDto.Absent, item.margin)
            assertEquals(OwnerFieldDto.Absent, item.productionCost)
            assertEquals(OwnerFieldDto.Absent, item.mainSupplier?.cost)
        }

    @Test
    fun aNullTheOwnerSawIsNotAbsent() =
        runTest {
            // The owner with nothing recorded: the field is there, with null.
            val api =
                catalogApi {
                    respondJson(
                        itemJson(
                            extra = ""","cost": null, "margin": null, "productionCost": null""",
                            supplierExtra = ""","cost": null""",
                        ),
                    )
                }

            val item = api.items.get(ITEM_ID)

            assertEquals(OwnerFieldDto.Null, item.cost)
            assertEquals(OwnerFieldDto.Null, item.margin)
            assertEquals(OwnerFieldDto.Null, item.productionCost)
            assertEquals(OwnerFieldDto.Null, item.mainSupplier?.cost)
        }

    @Test
    fun aValueTheOwnerSawIsKept() =
        runTest {
            val api =
                catalogApi {
                    respondJson(
                        itemJson(
                            extra =
                                ""","cost": {"cents": 41000, "currency": "BRL"},
                                  "margin": {"cents": 48000, "basisPoints": 5393}""",
                            supplierExtra = ""","cost": {"cents": 41000, "currency": "BRL"}""",
                        ),
                    )
                }

            val item = api.items.get(ITEM_ID)

            assertEquals(OwnerFieldDto.Value(MoneyDto(41000, "BRL")), item.cost)
            assertEquals(OwnerFieldDto.Value(MarginDto(48000, 5393)), item.margin)
            assertEquals(OwnerFieldDto.Value(MoneyDto(41000, "BRL")), item.mainSupplier?.cost)
        }

    @Test
    fun listRowsAndKitsFollowTheSameRule() =
        runTest {
            val rows =
                catalogApi {
                    respondJson("""{"items": [${itemSummaryJson()}, ${itemSummaryJson(""","cost": null""")}]}""")
                }.items.list(null, null, false, null, 20)
            val kit = catalogApi { respondJson(kitJson(""","cost": null, "margin": null""")) }.kits.get(KIT_ID)

            assertEquals(OwnerFieldDto.Absent, rows.items[0].cost)
            assertEquals(OwnerFieldDto.Null, rows.items[1].cost)
            assertEquals(OwnerFieldDto.Absent, rows.items[1].margin)
            assertEquals(OwnerFieldDto.Null, kit.cost)
            assertEquals(OwnerFieldDto.Null, kit.margin)
        }

    @Test
    fun theCacheFormKeepsAbsentNullAndValueApart() =
        runTest {
            // The offline cache stores what a DTO encodes to; reading it back must not turn a hidden cost into null.
            val hidden = catalogApi { respondJson(itemJson()) }.items.get(ITEM_ID)
            val nothing = catalogApi { respondJson(itemJson(""","cost": null""")) }.items.get(ITEM_ID)
            val recorded =
                catalogApi { respondJson(itemJson(""","cost": {"cents": 5, "currency": "BRL"}""")) }.items.get(ITEM_ID)

            fun roundTrip(item: ItemDto): ItemDto =
                ApiPayloadJson.decodeFromString<ItemDto>(ApiPayloadJson.encodeToString(item))

            assertFalse("\"cost\"" in ApiPayloadJson.encodeToString(hidden), "a hidden cost is not written")
            assertEquals(hidden, roundTrip(hidden))
            assertEquals(OwnerFieldDto.Null, roundTrip(nothing).cost)
            assertEquals(OwnerFieldDto.Value(MoneyDto(5, "BRL")), roundTrip(recorded).cost)
        }
}
