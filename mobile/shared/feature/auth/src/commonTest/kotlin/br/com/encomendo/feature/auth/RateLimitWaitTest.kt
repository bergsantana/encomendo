package br.com.encomendo.feature.auth

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

class RateLimitWaitTest {
    // Same thresholds as the web's errorMessage.ts, so both apps tell the same wait.

    @Test
    fun waitsUnderAMinuteStayInSeconds() {
        assertNull(minutesToWait(1))
        assertNull(minutesToWait(59))
    }

    @Test
    fun longerWaitsRoundUpToWholeMinutes() {
        // Rounding down would send the user back before the limit resets.
        assertEquals(1, minutesToWait(60))
        assertEquals(2, minutesToWait(61))
        assertEquals(60, minutesToWait(3600))
    }
}
