package br.com.encomendo.feature.members

import kotlinx.datetime.TimeZone
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.time.Instant

class PtBrDateTest {
    @Test
    fun anExpiryShowsAsDayMonthYear() {
        // "Vale até" in dd/mm/aaaa, as the web's formatDate.
        assertEquals("02/12/2026", ptBrDate(Instant.parse("2026-12-02T15:00:00Z"), TimeZone.of("America/Sao_Paulo")))
    }

    @Test
    fun theDateIsTheOneOnThePhonesClock() {
        // 02:00 UTC is still the evening before in São Paulo.
        assertEquals("01/12/2026", ptBrDate(Instant.parse("2026-12-02T02:00:00Z"), TimeZone.of("America/Sao_Paulo")))
    }
}
