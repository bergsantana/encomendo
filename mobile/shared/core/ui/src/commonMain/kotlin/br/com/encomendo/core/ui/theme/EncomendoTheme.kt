package br.com.encomendo.core.ui.theme

import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable

private val colorScheme =
    lightColorScheme(
        primary = EncomendoColors.Primary,
        onPrimary = EncomendoColors.Surface,
        background = EncomendoColors.Ground,
        onBackground = EncomendoColors.Ink,
        surface = EncomendoColors.Surface,
        onSurface = EncomendoColors.Ink,
        onSurfaceVariant = EncomendoColors.InkSecondary,
        outline = EncomendoColors.SecondaryBorder,
        outlineVariant = EncomendoColors.Hairline,
    )

/** EncomendoTheme wraps every screen of the management app. */
@Composable
fun EncomendoTheme(content: @Composable () -> Unit) {
    MaterialTheme(colorScheme = colorScheme, content = content)
}
