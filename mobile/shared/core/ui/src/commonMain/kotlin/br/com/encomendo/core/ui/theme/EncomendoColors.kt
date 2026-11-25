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

    // Status: always shown with an icon and a word (--enc-status-* in tokens.css).
    val StatusPlannedFill = Color(0xFFE5E9EE)
    val StatusPlannedText = Color(0xFF36414D)
    val StatusWaitingFill = Color(0xFFFDE9D6)
    val StatusWaitingText = Color(0xFF7A3300)
    val StatusAssemblyFill = Color(0xFFE1EDF9)
    val StatusAssemblyText = Color(0xFF0A4478)
    val StatusReadyFill = Color(0xFFDFF1E3)
    val StatusReadyText = Color(0xFF145226)
    val StatusAlertFill = Color(0xFFFBE3E3)
    val StatusAlertText = Color(0xFF981418)
    val StatusDeliveredFill = Color(0xFFE5EBE7)
    val StatusDeliveredText = Color(0xFF36423A)
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
    Planned(StatusColors(fill = EncomendoColors.StatusPlannedFill, text = EncomendoColors.StatusPlannedText)),
    Waiting(StatusColors(fill = EncomendoColors.StatusWaitingFill, text = EncomendoColors.StatusWaitingText)),
    Assembly(StatusColors(fill = EncomendoColors.StatusAssemblyFill, text = EncomendoColors.StatusAssemblyText)),
    Ready(StatusColors(fill = EncomendoColors.StatusReadyFill, text = EncomendoColors.StatusReadyText)),
    Alert(StatusColors(fill = EncomendoColors.StatusAlertFill, text = EncomendoColors.StatusAlertText)),
    Delivered(StatusColors(fill = EncomendoColors.StatusDeliveredFill, text = EncomendoColors.StatusDeliveredText)),
    Neutral(StatusColors(fill = EncomendoColors.Ground, text = EncomendoColors.InkSecondary)),
}
