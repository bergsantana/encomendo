package br.com.encomendo.feature.catalogo

import kotlin.test.Test
import kotlin.test.assertEquals

/**
 * "0 dias", "1 dia", "2 dias": Portuguese plural rules put 0 with the singular, the doc and the web do not.
 * Every "{n} dias" text (`lead_days`, `kit_store_default`, `kit_total_days`) picks its form through [daysQuantity],
 * which is what these tests pin; Compose resources cannot be rendered in a host test (they need the Android
 * resources), so the plurals' own words are proven on the device.
 */
class DaysTextTest {
    @Test
    fun zeroTakesTheFormOfManyDays() {
        assertEquals(2, daysQuantity(0), "0 dias")
    }

    @Test
    fun onlyOneDayIsSingular() {
        assertEquals(1, daysQuantity(1), "1 dia")
    }

    @Test
    fun everyOtherCountTakesTheFormOfManyDays() {
        assertEquals(2, daysQuantity(2), "2 dias")
        assertEquals(2, daysQuantity(60))
    }
}
