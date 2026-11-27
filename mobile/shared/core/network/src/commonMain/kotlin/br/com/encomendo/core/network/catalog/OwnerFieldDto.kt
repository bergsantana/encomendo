package br.com.encomendo.core.network.catalog

import kotlinx.serialization.ExperimentalSerializationApi
import kotlinx.serialization.KSerializer
import kotlinx.serialization.SerializationException
import kotlinx.serialization.builtins.serializer
import kotlinx.serialization.descriptors.SerialDescriptor
import kotlinx.serialization.encoding.Decoder
import kotlinx.serialization.encoding.Encoder
import kotlinx.serialization.json.JsonDecoder
import kotlinx.serialization.json.JsonNull

/**
 * OwnerFieldDto is a field with three states, which a nullable cannot tell
 * apart (ADR 0025): [Absent] (the key is not in the JSON), [Null] (the key is
 * there with `null`) and [Value]. Costs and margins are [Absent] for every
 * role but the owner, and [Null] for the owner when nothing is recorded.
 * In a write, [Absent] leaves the cost out so the server keeps it, [Null]
 * clears it and [Value] sets it.
 *
 * A property of this type must default to [Absent], so decoding a missing key
 * gives [Absent] and encoding it leaves the key out (`encodeDefaults` is off).
 */
sealed interface OwnerFieldDto<out T> {
    /** The key is not there. */
    data object Absent : OwnerFieldDto<Nothing>

    /** The key is there with `null`. */
    data object Null : OwnerFieldDto<Nothing>

    /** The key is there with a value. */
    data class Value<out T>(
        val value: T,
    ) : OwnerFieldDto<T>
}

/** Reads and writes an [OwnerFieldDto] whose value is [inner]; see [OwnerFieldDto] for the three states. */
class OwnerFieldSerializer<T>(
    private val inner: KSerializer<T>,
) : KSerializer<OwnerFieldDto<T>> {
    override val descriptor: SerialDescriptor = inner.descriptor

    override fun deserialize(decoder: Decoder): OwnerFieldDto<T> {
        val json = decoder as? JsonDecoder ?: throw SerializationException("owner-only fields are JSON only")
        // A JSON null reaches here, not a nullable wrapper, because the property type is not nullable.
        val element = json.decodeJsonElement()
        if (element is JsonNull) return OwnerFieldDto.Null
        return OwnerFieldDto.Value(json.json.decodeFromJsonElement(inner, element))
    }

    // encodeNull is how a serializer writes the JSON null of "clear this cost"; the API is stable in practice.
    @OptIn(ExperimentalSerializationApi::class)
    override fun serialize(
        encoder: Encoder,
        value: OwnerFieldDto<T>,
    ) {
        when (value) {
            // The property default is not encoded, so this is reached only if a caller sets Absent explicitly
            // with encodeDefaults on; failing loudly beats writing a cost the user never set.
            OwnerFieldDto.Absent -> throw SerializationException("an absent field has no JSON form")
            OwnerFieldDto.Null -> encoder.encodeNull()
            is OwnerFieldDto.Value -> encoder.encodeSerializableValue(inner, value.value)
        }
    }
}

/** An owner-only [MoneyDto] (`cost`, `productionCost`). */
object MoneyFieldSerializer : KSerializer<OwnerFieldDto<MoneyDto>> by OwnerFieldSerializer(MoneyDto.serializer())

/** An owner-only [MarginDto] (`margin`). */
object MarginFieldSerializer : KSerializer<OwnerFieldDto<MarginDto>> by OwnerFieldSerializer(MarginDto.serializer())

/** An owner-only amount in cents sent in a write (`costCents`, `productionCostCents`). */
object CentsFieldSerializer : KSerializer<OwnerFieldDto<Long>> by OwnerFieldSerializer(Long.serializer())
