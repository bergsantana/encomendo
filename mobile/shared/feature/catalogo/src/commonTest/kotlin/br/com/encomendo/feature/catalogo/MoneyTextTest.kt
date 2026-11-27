package br.com.encomendo.feature.catalogo

import br.com.encomendo.core.domain.money.Money
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

class MoneyTextTest {
    @Test
    fun formatsCentsAsBrazilianReais() {
        assertEquals("R$ 0,05", formatMoney(Money(5, "BRL")))
        assertEquals("R$ 890,00", formatMoney(Money(89_000, "BRL")))
        assertEquals("R$ 1.234.567,89", formatMoney(Money(123_456_789, "BRL")))
        assertEquals("-R$ 12,50", formatMoney(Money(-1_250, "BRL")))
    }

    @Test
    fun anotherCurrencyPrintsItsCode() {
        assertEquals("USD 10,00", formatMoney(Money(1_000, "USD")))
    }

    @Test
    fun theInputTextHasNoSymbolOrThousandsSeparator() {
        assertEquals("1234,50", moneyInputText(Money(123_450, "BRL")))
        assertEquals("0,07", moneyInputText(Money(7, "BRL")))
    }

    @Test
    fun readsWhatAPersonTypes() {
        assertEquals(12_345, parseCents("123,45"))
        assertEquals(123_450, parseCents("1.234,50"))
        assertEquals(1_250, parseCents("12,5"))
        assertEquals(1_250, parseCents("12.50"))
        assertEquals(1_200, parseCents("12"))
        assertEquals(123_400, parseCents("1.234"))
        assertEquals(5, parseCents("0,05"))
        assertEquals(5, parseCents(",05"))
        assertEquals(8_900, parseCents(" R$ 89 "))
    }

    @Test
    fun refusesWhatIsNotAnAmount() {
        assertNull(parseCents(""))
        assertNull(parseCents("abc"))
        assertNull(parseCents("-5"))
        assertNull(parseCents("1,234"))
        assertNull(parseCents("12,"))
        assertNull(parseCents("9".repeat(20)))
    }

    @Test
    fun formatsAMarginsPercentage() {
        assertEquals("30%", formatBasisPoints(3_000))
        assertEquals("33,5%", formatBasisPoints(3_350))
        assertEquals("33,33%", formatBasisPoints(3_333))
        assertEquals("0,5%", formatBasisPoints(50))
        assertEquals("-12%", formatBasisPoints(-1_200))
        assertEquals("-0,5%", formatBasisPoints(-50))
    }
}
