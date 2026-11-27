package br.com.encomendo.core.data.catalog

import br.com.encomendo.core.domain.catalog.CostChange
import br.com.encomendo.core.domain.catalog.CustomizationOption
import br.com.encomendo.core.domain.catalog.Item
import br.com.encomendo.core.domain.catalog.ItemDraft
import br.com.encomendo.core.domain.catalog.ItemSummary
import br.com.encomendo.core.domain.catalog.OptionChoice
import br.com.encomendo.core.domain.catalog.OptionChoiceDraft
import br.com.encomendo.core.domain.catalog.OptionDraft
import br.com.encomendo.core.domain.catalog.SourcingMode
import br.com.encomendo.core.domain.catalog.SupplierItem
import br.com.encomendo.core.domain.catalog.SupplierLinkDraft
import br.com.encomendo.core.network.catalog.ItemDto
import br.com.encomendo.core.network.catalog.ItemSummaryDto
import br.com.encomendo.core.network.catalog.ItemWriteDto
import br.com.encomendo.core.network.catalog.OptionChoiceWriteDto
import br.com.encomendo.core.network.catalog.OptionDto
import br.com.encomendo.core.network.catalog.OptionWriteDto
import br.com.encomendo.core.network.catalog.OwnerFieldDto
import br.com.encomendo.core.network.catalog.PhotoDto
import br.com.encomendo.core.network.catalog.SupplierItemDto
import br.com.encomendo.core.network.catalog.SupplierLinkWriteDto

private fun SupplierItemDto.toSupplierItem(): SupplierItem =
    SupplierItem(
        supplierId = supplierId,
        supplierName = supplierName,
        leadDays = leadDays,
        cost = cost.toCost(),
    )

private fun String.toSourcingMode(): SourcingMode = SourcingMode.fromKey(this) ?: unusable()

internal fun ItemSummaryDto.toItemSummary(): ItemSummary =
    ItemSummary(
        id = id,
        name = name,
        categoryId = categoryId,
        sourcingMode = sourcingMode.toSourcingMode(),
        price = price.toMoney(),
        readyDays = readyDays,
        mainSupplierName = mainSupplierName,
        published = published,
        archived = archived,
        cover = cover?.toPhoto(),
        version = version,
        cost = cost.toCost(),
        margin = margin.toMargin(),
    )

internal fun ItemDto.toItem(): Item =
    Item(
        id = id,
        name = name,
        description = description,
        categoryId = categoryId,
        sourcingMode = sourcingMode.toSourcingMode(),
        price = price.toMoney(),
        productionDays = productionDays,
        readyDays = readyDays,
        mainSupplier = mainSupplier?.toSupplierItem(),
        backupSupplier = backupSupplier?.toSupplierItem(),
        attributeValueIds = attributeValueIds,
        options = options.map(OptionDto::toOption),
        photos = photos.map(PhotoDto::toPhoto),
        published = published,
        archived = archived,
        version = version,
        productionCost = productionCost.toCost(),
        cost = cost.toCost(),
        margin = margin.toMargin(),
    )

private fun OptionDto.toOption(): CustomizationOption =
    CustomizationOption(
        id = id,
        name = name,
        required = required,
        choices =
            choices.map {
                OptionChoice(
                    id = it.id,
                    label = it.label,
                    extraPrice = it.extraPrice.toMoney(),
                    extraDays = it.extraDays,
                )
            },
    )

/** Maps a draft to the write body; [version] is set when replacing. A cost left as [CostChange.Keep] is not sent. */
internal fun ItemDraft.toWrite(version: Int?): ItemWriteDto =
    ItemWriteDto(
        name = name,
        description = description,
        categoryId = categoryId,
        sourcingMode = sourcingMode.key,
        priceCents = priceCents,
        productionDays = productionDays,
        mainSupplier = mainSupplier?.toWrite(),
        backupSupplier = backupSupplier?.toWrite(),
        attributeValueIds = attributeValueIds,
        options = options.map(OptionDraft::toWrite),
        published = published,
        productionCostCents = productionCost.toField(),
        version = version,
    )

private fun SupplierLinkDraft.toWrite(): SupplierLinkWriteDto =
    SupplierLinkWriteDto(supplierId = supplierId, leadDays = leadDays, costCents = cost.toField())

private fun CostChange.toField(): OwnerFieldDto<Long> =
    when (this) {
        CostChange.Keep -> OwnerFieldDto.Absent
        CostChange.Clear -> OwnerFieldDto.Null
        is CostChange.Set -> OwnerFieldDto.Value(cents)
    }

private fun OptionDraft.toWrite(): OptionWriteDto =
    OptionWriteDto(name = name, required = required, choices = choices.map(OptionChoiceDraft::toWrite), id = id)

private fun OptionChoiceDraft.toWrite(): OptionChoiceWriteDto =
    OptionChoiceWriteDto(label = label, extraPriceCents = extraPriceCents, extraDays = extraDays, id = id)
