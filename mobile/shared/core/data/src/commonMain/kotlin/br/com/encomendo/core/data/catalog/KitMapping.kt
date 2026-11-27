package br.com.encomendo.core.data.catalog

import br.com.encomendo.core.domain.catalog.Kit
import br.com.encomendo.core.domain.catalog.KitDays
import br.com.encomendo.core.domain.catalog.KitDraft
import br.com.encomendo.core.domain.catalog.KitItem
import br.com.encomendo.core.domain.catalog.KitSummary
import br.com.encomendo.core.domain.catalog.SwapDiscount
import br.com.encomendo.core.network.catalog.KitDto
import br.com.encomendo.core.network.catalog.KitItemDto
import br.com.encomendo.core.network.catalog.KitItemWriteDto
import br.com.encomendo.core.network.catalog.KitSummaryDto
import br.com.encomendo.core.network.catalog.KitWriteDto
import br.com.encomendo.core.network.catalog.PhotoDto

internal fun KitSummaryDto.toKitSummary(): KitSummary =
    KitSummary(
        id = id,
        name = name,
        itemCount = itemCount,
        price = price.toMoney(),
        singleItemsTotal = singleItemsTotal.toMoney(),
        totalDays = totalDays,
        published = published,
        archived = archived,
        cover = cover?.toPhoto(),
        version = version,
        cost = cost.toCost(),
        margin = margin.toMargin(),
    )

internal fun KitDto.toKit(): Kit =
    Kit(
        id = id,
        name = name,
        description = description,
        price = price.toMoney(),
        singleItemsTotal = singleItemsTotal.toMoney(),
        discount = discount.toMoney(),
        swapDiscount = SwapDiscount.fromKey(swapDiscount) ?: unusable(),
        assemblyDays = assemblyDays,
        safetyMarginDays = safetyMarginDays,
        days =
            KitDays(
                slowestItem = days.slowestItem,
                assembly = days.assembly,
                safetyMargin = days.safetyMargin,
                total = days.total,
            ),
        items = items.map(KitItemDto::toKitItem),
        attributeValueIds = attributeValueIds,
        photos = photos.map(PhotoDto::toPhoto),
        published = published,
        archived = archived,
        version = version,
        cost = cost.toCost(),
        margin = margin.toMargin(),
    )

private fun KitItemDto.toKitItem(): KitItem =
    KitItem(
        itemId = itemId,
        itemName = itemName,
        quantity = quantity,
        swappable = swappable,
        alternativeItemIds = alternativeItemIds,
        unitPrice = unitPrice.toMoney(),
        readyDays = readyDays,
        archived = archived,
    )

internal fun KitDraft.toWrite(version: Int?): KitWriteDto =
    KitWriteDto(
        name = name,
        description = description,
        priceCents = priceCents,
        swapDiscount = swapDiscount.key,
        assemblyDays = assemblyDays,
        safetyMarginDays = safetyMarginDays,
        items =
            items.map {
                KitItemWriteDto(
                    itemId = it.itemId,
                    quantity = it.quantity,
                    swappable = it.swappable,
                    alternativeItemIds = it.alternativeItemIds,
                )
            },
        attributeValueIds = attributeValueIds,
        published = published,
        version = version,
    )
