package br.com.encomendo.core.domain.catalog

/** Category groups the store's items (`Category` in docs/api/openapi.yaml); an item has at most one. */
data class Category(
    /** The category's id (a UUID). */
    val id: String,
    /** The name the store gave it, such as "Bancos". */
    val name: String,
    /** Its place in the store's order, from 0. */
    val position: Int,
)

/** Attribute is an item field such as "Serve em", with the values items and kits pick (ADR 0025). */
data class Attribute(
    /** The field's id (a UUID). */
    val id: String,
    /** The field's name. */
    val name: String,
    /** Its place in the store's order, from 0. */
    val position: Int,
    /** Its values, in the store's order. */
    val values: List<AttributeValue>,
)

/** AttributeValue is one choice of an [Attribute], such as "CB 500X". */
data class AttributeValue(
    /** The value's id (a UUID). */
    val id: String,
    /** The text the store gave it. */
    val label: String,
    /** How many items and kits, archived ones included, picked it; the app asks before deleting a used value. */
    val usageCount: Int,
)

/** AttributeDraft is a field as the user edited it, ready to save. */
data class AttributeDraft(
    /** The field's name. */
    val name: String,
    /** The values in the store's order; a value keeping its [AttributeValueDraft.id] keeps the items that use it. */
    val values: List<AttributeValueDraft>,
)

/** AttributeValueDraft is one value of an [AttributeDraft]. */
data class AttributeValueDraft(
    /** The id of the existing value this one keeps; null for a new value. */
    val id: String?,
    /** The text of the value. */
    val label: String,
)
