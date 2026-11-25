package br.com.encomendo.core.ui.theme

import androidx.compose.ui.graphics.Color
import kotlin.test.Test
import kotlin.test.assertEquals

class StatusColorsTest {
    @Test
    fun matchesTheWebStatusTokens() {
        // Same values as --enc-status-*-fill / -text in web/packages/ui/src/tokens.css.
        val expected =
            mapOf(
                StatusTone.Planned to StatusColors(fill = Color(0xFFE5E9EE), text = Color(0xFF36414D)),
                StatusTone.Waiting to StatusColors(fill = Color(0xFFFDE9D6), text = Color(0xFF7A3300)),
                StatusTone.Assembly to StatusColors(fill = Color(0xFFE1EDF9), text = Color(0xFF0A4478)),
                StatusTone.Ready to StatusColors(fill = Color(0xFFDFF1E3), text = Color(0xFF145226)),
                StatusTone.Alert to StatusColors(fill = Color(0xFFFBE3E3), text = Color(0xFF981418)),
                StatusTone.Delivered to StatusColors(fill = Color(0xFFE5EBE7), text = Color(0xFF36423A)),
                StatusTone.Neutral to StatusColors(fill = Color(0xFFF5F4F0), text = Color(0xFF545860)),
            )

        assertEquals(expected, StatusTone.entries.associateWith { it.colors })
    }
}
