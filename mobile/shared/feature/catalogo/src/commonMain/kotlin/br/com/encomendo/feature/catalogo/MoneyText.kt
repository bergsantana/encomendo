package br.com.encomendo.feature.catalogo

import br.com.encomendo.core.domain.money.Money

private const val CENTS_PER_UNIT = 100L
private const val GROUP = 3
private const val BASIS_POINTS_PER_PERCENT = 100
private const val MAX_CENTS = 100_000_000_000L
private const val MAX_FRACTION_DIGITS = 2

/**
 * Returns [money] the way a Brazilian reads it: "R$ 1.234,56". Only formats what the API sent: the app never adds,
 * converts or rounds amounts (mobile/CLAUDE.md). Other currencies print their ISO code in place of "R$".
 */
internal fun formatMoney(money: Money): String {
    val sign = if (money.cents < 0) "-" else ""
    val absolute = if (money.cents < 0) -money.cents else money.cents
    val units =
        (absolute / CENTS_PER_UNIT)
            .toString()
            .reversed()
            .chunked(GROUP)
            .joinToString(".")
            .reversed()
    val cents = (absolute % CENTS_PER_UNIT).toString().padStart(MAX_FRACTION_DIGITS, '0')
    val symbol = if (money.currency == "BRL") "R$" else money.currency
    return "$sign$symbol $units,$cents"
}

/** Returns [money] as the text an input field starts with: "1234,56", no symbol and no thousands separator. */
internal fun moneyInputText(money: Money): String {
    val units = money.cents / CENTS_PER_UNIT
    val cents = (money.cents % CENTS_PER_UNIT).toString().padStart(MAX_FRACTION_DIGITS, '0')
    return "$units,$cents"
}

/**
 * Returns a margin's basis points as a percentage, "30%" or "33,5%". Formatting only: the server computed it.
 */
internal fun formatBasisPoints(basisPoints: Int): String {
    val whole = basisPoints / BASIS_POINTS_PER_PERCENT
    val rest = kotlin.math.abs(basisPoints % BASIS_POINTS_PER_PERCENT)
    val sign = if (basisPoints < 0 && whole == 0) "-" else ""
    val fraction = rest.toString().padStart(MAX_FRACTION_DIGITS, '0').trimEnd('0')
    return if (fraction.isEmpty()) "$sign$whole%" else "$sign$whole,$fraction%"
}

/**
 * Reads what a person typed as an amount, "1.234,56", "12,5", "12" or "12.50", into cents. Returns null for
 * anything else, for more than two decimals and for an amount past what the API accepts. Reading what was typed is
 * not a calculation: the cents are exactly what the person wrote.
 */
internal fun parseCents(text: String): Long? {
    val cleaned = text.trim().removePrefix("R$").trim()
    val decimalSeparator = decimalSeparatorOf(cleaned) ?: return wholeCents(cleaned)
    val units = cleaned.substringBeforeLast(decimalSeparator).replace(".", "").replace(",", "")
    val fraction = cleaned.substringAfterLast(decimalSeparator)
    val valid = fraction.length in 1..MAX_FRACTION_DIGITS && fraction.all(Char::isDigit)
    return if (valid) centsOf(units.ifEmpty { "0" }, fraction.padEnd(MAX_FRACTION_DIGITS, '0')) else null
}

/** The character that starts the decimals: the last comma, or a lone dot with one or two digits after it. */
private fun decimalSeparatorOf(text: String): Char? =
    when {
        ',' in text -> ','
        text.count { it == '.' } == 1 && text.substringAfter('.').length <= MAX_FRACTION_DIGITS -> '.'
        else -> null
    }

private fun wholeCents(text: String): Long? = centsOf(text.replace(".", ""), "00")

private fun centsOf(
    units: String,
    fraction: String,
): Long? {
    if (units.isEmpty() || !units.all(Char::isDigit) || units.length > MAX_UNIT_DIGITS) return null
    val cents = units.toLong() * CENTS_PER_UNIT + fraction.toLong()
    return cents.takeIf { it <= MAX_CENTS }
}

private const val MAX_UNIT_DIGITS = 12
