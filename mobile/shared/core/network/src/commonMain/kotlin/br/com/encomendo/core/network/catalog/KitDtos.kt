package br.com.encomendo.core.network.catalog

import kotlinx.serialization.Serializable

/** KitSummaryDto is `KitSummary`; [cost] and [margin] are [OwnerFieldDto.Absent] for every role but the owner. */
@Serializable
data class KitSummaryDto(
    val id: String,
    val name: String,
    val itemCount: Int,
    val price: MoneyDto,
    val singleItemsTotal: MoneyDto,
    val totalDays: Int,
    val published: Boolean,
    val archived: Boolean,
    val cover: PhotoDto? = null,
    val version: Int,
    @Serializable(with = MoneyFieldSerializer::class)
    val cost: OwnerFieldDto<MoneyDto> = OwnerFieldDto.Absent,
    @Serializable(with = MarginFieldSerializer::class)
    val margin: OwnerFieldDto<MarginDto> = OwnerFieldDto.Absent,
)

/** KitPageDto is `KitPage`; [nextCursor] is absent on the last page. */
@Serializable
data class KitPageDto(
    val items: List<KitSummaryDto>,
    val nextCursor: String? = null,
)

/** KitDaysDto is `KitDays`: "Prazo calculado". */
@Serializable
data class KitDaysDto(
    val slowestItem: Int,
    val assembly: Int,
    val safetyMargin: Int,
    val total: Int,
)

/** KitItemDto is `KitItem`. */
@Serializable
data class KitItemDto(
    val itemId: String,
    val itemName: String,
    val quantity: Int,
    val swappable: Boolean,
    val alternativeItemIds: List<String>,
    val unitPrice: MoneyDto,
    val readyDays: Int,
    val archived: Boolean,
)

/** KitDto is `Kit`; [cost] and [margin] are [OwnerFieldDto.Absent] for every role but the owner. */
@Serializable
data class KitDto(
    val id: String,
    val name: String,
    val description: String,
    val price: MoneyDto,
    val singleItemsTotal: MoneyDto,
    val discount: MoneyDto,
    val swapDiscount: String,
    val assemblyDays: Int? = null,
    val safetyMarginDays: Int? = null,
    val days: KitDaysDto,
    val items: List<KitItemDto>,
    val attributeValueIds: List<String>,
    val photos: List<PhotoDto>,
    val published: Boolean,
    val archived: Boolean,
    val version: Int,
    @Serializable(with = MoneyFieldSerializer::class)
    val cost: OwnerFieldDto<MoneyDto> = OwnerFieldDto.Absent,
    @Serializable(with = MarginFieldSerializer::class)
    val margin: OwnerFieldDto<MarginDto> = OwnerFieldDto.Absent,
)

/** KitItemWriteDto is an item of a `KitInput`. */
@Serializable
data class KitItemWriteDto(
    val itemId: String,
    val quantity: Int,
    val swappable: Boolean,
    val alternativeItemIds: List<String>,
)

/**
 * KitWriteDto is `KitInput`, or `ReplaceKitRequest` when [version] is set. The assembly and safety margin
 * days are sent even when null: null means the store's default.
 */
@Serializable
data class KitWriteDto(
    val name: String,
    val description: String,
    val priceCents: Long,
    val swapDiscount: String,
    val assemblyDays: Int?,
    val safetyMarginDays: Int?,
    val items: List<KitItemWriteDto>,
    val attributeValueIds: List<String>,
    val published: Boolean,
    val version: Int? = null,
)
