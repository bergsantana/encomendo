package br.com.encomendo.core.domain.catalog

import br.com.encomendo.core.domain.session.Role

/**
 * OwnerField is a cost or margin the API sends to the owner only (ADR 0025).
 * It tells apart two things a nullable cannot: [Hidden], the field was absent
 * because the role may not see costs, and [Shown] with a null [Shown.value],
 * the owner sees the field but nothing is recorded. Screens show a cost or
 * margin column only for [Shown].
 */
sealed interface OwnerField<out T> {
    /** The API left the field out: this role does not see costs. Show no cost column. */
    data object Hidden : OwnerField<Nothing>

    /** The API sent the field. [value] is null when no cost is recorded. */
    data class Shown<out T>(
        val value: T?,
    ) : OwnerField<T>
}

/** True when the API sent the field, so the screen shows its column. */
val OwnerField<*>.isShown: Boolean get() = this is OwnerField.Shown

/** The recorded value, or null when the field is hidden or nothing is recorded. */
val <T> OwnerField<T>.valueOrNull: T? get() = (this as? OwnerField.Shown)?.value

/**
 * Returns true for the role that sees costs and margins: the owner only
 * (`identity.SeesCosts` in the backend, ADR 0025). The app uses it to decide
 * what it may keep on the device; what it shows depends on the API sending
 * the field.
 */
fun Role.seesCosts(): Boolean = this == Role.Owner

/**
 * CostChange is what the user did to one cost field of a write: left it
 * alone, cleared it or set it. A write from any role but the owner is always
 * [Keep], and a [Keep] is left out of the request entirely, so the server
 * keeps the stored cost (ADR 0025).
 */
sealed interface CostChange {
    /** Leave the stored cost as it is; the field is not sent. */
    data object Keep : CostChange

    /** Record that there is no cost; the field is sent as null. */
    data object Clear : CostChange

    /** Record [cents] as the cost, in the store's currency. */
    data class Set(
        val cents: Long,
    ) : CostChange
}
