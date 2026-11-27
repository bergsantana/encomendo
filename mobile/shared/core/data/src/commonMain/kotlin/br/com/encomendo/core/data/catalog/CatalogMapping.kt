package br.com.encomendo.core.data.catalog

import br.com.encomendo.core.domain.catalog.Attribute
import br.com.encomendo.core.domain.catalog.AttributeDraft
import br.com.encomendo.core.domain.catalog.AttributeValue
import br.com.encomendo.core.domain.catalog.Category
import br.com.encomendo.core.domain.catalog.Margin
import br.com.encomendo.core.domain.catalog.OwnerField
import br.com.encomendo.core.domain.catalog.Photo
import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.domain.money.Money
import br.com.encomendo.core.network.catalog.AttributeDto
import br.com.encomendo.core.network.catalog.AttributeValueWriteDto
import br.com.encomendo.core.network.catalog.AttributeWriteDto
import br.com.encomendo.core.network.catalog.CategoryDto
import br.com.encomendo.core.network.catalog.MarginDto
import br.com.encomendo.core.network.catalog.MoneyDto
import br.com.encomendo.core.network.catalog.OwnerFieldDto
import br.com.encomendo.core.network.catalog.PhotoDto
import kotlin.time.Instant

// An answer that breaks the contract is a failure the screen cannot say more about, as a body that does not decode.
internal fun unusable(): Nothing = throw Failure.Unexpected(status = OK, code = "unknown")

private const val OK = 200

internal fun MoneyDto.toMoney(): Money = Money(cents = cents, currency = currency)

/** Keeps the three states of an owner-only field: left out is hidden, null is shown without a value. */
internal fun <D, T> OwnerFieldDto<D>.toOwnerField(map: (D) -> T): OwnerField<T> =
    when (this) {
        OwnerFieldDto.Absent -> OwnerField.Hidden
        OwnerFieldDto.Null -> OwnerField.Shown(null)
        is OwnerFieldDto.Value -> OwnerField.Shown(map(value))
    }

internal fun OwnerFieldDto<MoneyDto>.toCost(): OwnerField<Money> = toOwnerField(MoneyDto::toMoney)

internal fun OwnerFieldDto<MarginDto>.toMargin(): OwnerField<Margin> =
    toOwnerField { Margin(cents = it.cents, basisPoints = it.basisPoints) }

internal fun PhotoDto.toPhoto(): Photo =
    Photo(
        id = id,
        url = url,
        expiresAt = instantOrUnusable(expiresAt),
        position = position,
    )

internal fun instantOrUnusable(text: String): Instant =
    try {
        Instant.parse(text)
    } catch (_: IllegalArgumentException) {
        unusable()
    }

internal fun CategoryDto.toCategory(): Category = Category(id = id, name = name, position = position)

internal fun AttributeDto.toAttribute(): Attribute =
    Attribute(
        id = id,
        name = name,
        position = position,
        values = values.map { AttributeValue(id = it.id, label = it.label, usageCount = it.usageCount) },
    )

internal fun AttributeDraft.toWrite(): AttributeWriteDto =
    AttributeWriteDto(name = name, values = values.map { AttributeValueWriteDto(label = it.label, id = it.id) })
