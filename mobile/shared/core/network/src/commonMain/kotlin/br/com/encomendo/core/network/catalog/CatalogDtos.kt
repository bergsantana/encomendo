package br.com.encomendo.core.network.catalog

import kotlinx.serialization.Serializable

/** MoneyDto is `Money` in docs/api/openapi.yaml: integer cents and a currency code. */
@Serializable
data class MoneyDto(
    val cents: Long,
    val currency: String,
)

/** MarginDto is `Margin`: price minus cost, and its share of the price in hundredths of a percent. */
@Serializable
data class MarginDto(
    val cents: Long,
    val basisPoints: Int,
)

/** CategoryDto is `Category`. */
@Serializable
data class CategoryDto(
    val id: String,
    val name: String,
    val position: Int,
)

/** CategoryListDto is `CategoryList`: every category, with no paging. */
@Serializable
data class CategoryListDto(
    val items: List<CategoryDto>,
)

/** AttributeValueDto is `AttributeValue`. */
@Serializable
data class AttributeValueDto(
    val id: String,
    val label: String,
    val usageCount: Int,
)

/** AttributeDto is `Attribute`: an item field and its values. */
@Serializable
data class AttributeDto(
    val id: String,
    val name: String,
    val position: Int,
    val values: List<AttributeValueDto>,
)

/** AttributeListDto is `AttributeList`. */
@Serializable
data class AttributeListDto(
    val items: List<AttributeDto>,
)

/** AttributeValueWriteDto is one value of an `AttributeInput`; [id] is left out for a new value. */
@Serializable
data class AttributeValueWriteDto(
    val label: String,
    val id: String? = null,
)

/** AttributeWriteDto is `AttributeInput`. */
@Serializable
data class AttributeWriteDto(
    val name: String,
    val values: List<AttributeValueWriteDto>,
)

/** PhotoDto is `Photo`; [expiresAt] is an RFC 3339 date-time. */
@Serializable
data class PhotoDto(
    val id: String,
    val url: String,
    val expiresAt: String,
    val position: Int,
)

/** PhotoListDto is `PhotoList`. */
@Serializable
data class PhotoListDto(
    val items: List<PhotoDto>,
)

/** PhotoUploadDto is `PhotoUpload`: where and how to upload one photo. [expiresAt] is an RFC 3339 date-time. */
@Serializable
data class PhotoUploadDto(
    val key: String,
    val uploadUrl: String,
    val method: String,
    val headers: Map<String, String>,
    val expiresAt: String,
)

/** SupplierItemDto is `SupplierItem`; [cost] is [OwnerFieldDto.Absent] for every role but the owner. */
@Serializable
data class SupplierItemDto(
    val supplierId: String,
    val supplierName: String,
    val leadDays: Int,
    @Serializable(with = MoneyFieldSerializer::class)
    val cost: OwnerFieldDto<MoneyDto> = OwnerFieldDto.Absent,
)

/** SupplierLinkWriteDto is `SupplierItemInput`; [costCents] stays [OwnerFieldDto.Absent] unless the owner set it. */
@Serializable
data class SupplierLinkWriteDto(
    val supplierId: String,
    val leadDays: Int,
    @Serializable(with = CentsFieldSerializer::class)
    val costCents: OwnerFieldDto<Long> = OwnerFieldDto.Absent,
)

/** SupplierDto is `Supplier`. */
@Serializable
data class SupplierDto(
    val id: String,
    val name: String,
    val whatsappPhone: String,
    val version: Int,
)

/** SupplierPageDto is one page of `GET /api/v1/admin/suppliers`; [nextCursor] is absent on the last page. */
@Serializable
data class SupplierPageDto(
    val items: List<SupplierDto>,
    val nextCursor: String? = null,
)

/** SupplierWriteDto is `SupplierInput`, or `ReplaceSupplierRequest` when [version] is set. */
@Serializable
data class SupplierWriteDto(
    val name: String,
    val whatsappPhone: String,
    val version: Int? = null,
)
