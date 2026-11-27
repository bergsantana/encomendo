package br.com.encomendo.core.network.catalog

import kotlinx.serialization.Serializable

/** ItemSummaryDto is `ItemSummary`; [cost] and [margin] are [OwnerFieldDto.Absent] for every role but the owner. */
@Serializable
data class ItemSummaryDto(
    val id: String,
    val name: String,
    val categoryId: String? = null,
    val sourcingMode: String,
    val price: MoneyDto,
    val readyDays: Int,
    val mainSupplierName: String? = null,
    val published: Boolean,
    val archived: Boolean,
    val cover: PhotoDto? = null,
    val version: Int,
    @Serializable(with = MoneyFieldSerializer::class)
    val cost: OwnerFieldDto<MoneyDto> = OwnerFieldDto.Absent,
    @Serializable(with = MarginFieldSerializer::class)
    val margin: OwnerFieldDto<MarginDto> = OwnerFieldDto.Absent,
)

/** ItemPageDto is `ItemPage`; [nextCursor] is absent on the last page. */
@Serializable
data class ItemPageDto(
    val items: List<ItemSummaryDto>,
    val nextCursor: String? = null,
)

/** OptionChoiceDto is a choice of a `CustomizationOption`. */
@Serializable
data class OptionChoiceDto(
    val id: String,
    val label: String,
    val extraPrice: MoneyDto,
    val extraDays: Int,
)

/** OptionDto is `CustomizationOption`. */
@Serializable
data class OptionDto(
    val id: String,
    val name: String,
    val required: Boolean,
    val choices: List<OptionChoiceDto>,
)

/** ItemDto is `Item`; the owner-only fields are [OwnerFieldDto.Absent] for every other role. */
@Serializable
data class ItemDto(
    val id: String,
    val name: String,
    val description: String,
    val categoryId: String? = null,
    val sourcingMode: String,
    val price: MoneyDto,
    val productionDays: Int? = null,
    val readyDays: Int,
    val mainSupplier: SupplierItemDto? = null,
    val backupSupplier: SupplierItemDto? = null,
    val attributeValueIds: List<String>,
    val options: List<OptionDto>,
    val photos: List<PhotoDto>,
    val published: Boolean,
    val archived: Boolean,
    val version: Int,
    @Serializable(with = MoneyFieldSerializer::class)
    val productionCost: OwnerFieldDto<MoneyDto> = OwnerFieldDto.Absent,
    @Serializable(with = MoneyFieldSerializer::class)
    val cost: OwnerFieldDto<MoneyDto> = OwnerFieldDto.Absent,
    @Serializable(with = MarginFieldSerializer::class)
    val margin: OwnerFieldDto<MarginDto> = OwnerFieldDto.Absent,
)

/** OptionChoiceWriteDto is a choice of a `CustomizationOptionInput`. */
@Serializable
data class OptionChoiceWriteDto(
    val label: String,
    val extraPriceCents: Long,
    val extraDays: Int,
    val id: String? = null,
)

/** OptionWriteDto is `CustomizationOptionInput`. */
@Serializable
data class OptionWriteDto(
    val name: String,
    val required: Boolean,
    val choices: List<OptionChoiceWriteDto>,
    val id: String? = null,
)

/**
 * ItemWriteDto is `ItemInput`, or `ReplaceItemRequest` when [version] is set. The cost fields default to
 * [OwnerFieldDto.Absent], which is left out of the body, so an admin's write carries no cost key at all.
 * The fields without a default are sent even when null: the contract requires them.
 */
@Serializable
data class ItemWriteDto(
    val name: String,
    val description: String,
    val categoryId: String?,
    val sourcingMode: String,
    val priceCents: Long,
    val productionDays: Int?,
    val mainSupplier: SupplierLinkWriteDto?,
    val backupSupplier: SupplierLinkWriteDto?,
    val attributeValueIds: List<String>,
    val options: List<OptionWriteDto>,
    val published: Boolean,
    @Serializable(with = CentsFieldSerializer::class)
    val productionCostCents: OwnerFieldDto<Long> = OwnerFieldDto.Absent,
    val version: Int? = null,
)
