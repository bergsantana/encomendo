package br.com.encomendo.core.domain.money

/**
 * Money is an amount in integer cents of [currency], never a float
 * (`Money` in docs/api/openapi.yaml). Formatting it is the UI layer's job;
 * the app never adds, converts or rounds amounts: totals, prices and
 * margins come from the API.
 */
data class Money(
    /** The amount in the currency's smallest unit; the API sends it as an int64. */
    val cents: Long,
    /** The ISO 4217 code, such as `BRL`. */
    val currency: String,
)
