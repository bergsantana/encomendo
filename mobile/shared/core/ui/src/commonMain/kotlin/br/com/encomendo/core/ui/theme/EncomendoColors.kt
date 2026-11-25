package br.com.encomendo.core.ui.theme

import androidx.compose.ui.graphics.Color

/**
 * Repair Shop design system colors, the same values as web/packages/ui/src/tokens.css.
 * Graphite does actions; color only means status; yellow is only for the logo,
 * selection and focus, never text and never status.
 */
object EncomendoColors {
    val Ground = Color(0xFFF5F4F0)
    val Surface = Color(0xFFFFFFFF)
    val Hairline = Color(0xFFE3E1DA)
    val Ink = Color(0xFF1B1D20)
    val InkSecondary = Color(0xFF545860)
    val Primary = Color(0xFF1B1D20)
    val SecondaryBorder = Color(0xFF7D796F)
    val Accent = Color(0xFFFFC800)
}

/** Fill and text color of one status family. */
data class StatusColors(
    val fill: Color,
    val text: Color,
)

/** Status color families from the design system, shared with web. */
enum class StatusTone(
    val colors: StatusColors,
) {
    Planned(StatusColors(fill = Color(0xFFE5E9EE), text = Color(0xFF36414D))),
    Waiting(StatusColors(fill = Color(0xFFFDE9D6), text = Color(0xFF7A3300))),
    Assembly(StatusColors(fill = Color(0xFFE1EDF9), text = Color(0xFF0A4478))),
    Ready(StatusColors(fill = Color(0xFFDFF1E3), text = Color(0xFF145226))),
    Alert(StatusColors(fill = Color(0xFFFBE3E3), text = Color(0xFF981418))),
    Delivered(StatusColors(fill = Color(0xFFE5EBE7), text = Color(0xFF36423A))),
    Neutral(StatusColors(fill = EncomendoColors.Ground, text = EncomendoColors.InkSecondary)),
}
