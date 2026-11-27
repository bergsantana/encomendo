package br.com.encomendo.feature.catalogo

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.Button
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.Typography
import androidx.compose.runtime.Composable
import androidx.compose.runtime.compositionLocalOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.role
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import br.com.encomendo.core.ui.PrimaryTarget
import br.com.encomendo.core.ui.TouchTarget
import br.com.encomendo.core.ui.failure.FailureMessage
import br.com.encomendo.core.ui.failure.text
import br.com.encomendo.feature.catalogo.resources.Res
import br.com.encomendo.feature.catalogo.resources.message_reload
import org.jetbrains.compose.resources.stringResource

private val WORKSHOP_BODY = 18.sp

/** CatalogSubject says whether a screen is about an item or a kit, for the few sentences that name which. */
internal enum class CatalogSubject {
    /** The Itens screens. */
    Item,

    /** The Kits screens. */
    Kit,
}

/** The subject of the screen being composed; the item screens are the default. */
internal val LocalCatalogSubject = compositionLocalOf { CatalogSubject.Item }

/** CatalogoTheme sets the body text of the Catálogo screens to the workshop size, 18 sp. */
@Composable
internal fun CatalogoTheme(content: @Composable () -> Unit) {
    val base = MaterialTheme.typography
    val workshop: Typography =
        base.copy(
            bodyLarge = base.bodyLarge.copy(fontSize = WORKSHOP_BODY),
            bodyMedium = base.bodyMedium.copy(fontSize = WORKSHOP_BODY),
            bodySmall = base.bodySmall.copy(fontSize = WORKSHOP_BODY),
            labelLarge = base.labelLarge.copy(fontSize = WORKSHOP_BODY),
        )
    MaterialTheme(
        colorScheme = MaterialTheme.colorScheme,
        shapes = MaterialTheme.shapes,
        typography = workshop,
        content = content,
    )
}

/** Read out once it appears or changes, without interrupting. */
internal fun Modifier.announced(): Modifier = semantics { liveRegion = LiveRegionMode.Polite }

/** The text of a failure above a form or a list, in the error color; [onReload] adds "Recarregar" for a conflict. */
@Composable
internal fun MessageText(
    message: CatalogMessage,
    modifier: Modifier = Modifier,
    onReload: (() -> Unit)? = null,
) {
    Column(modifier = modifier, verticalArrangement = Arrangement.spacedBy(8.dp)) {
        Text(
            text = message.text(),
            color = MaterialTheme.colorScheme.error,
            style = MaterialTheme.typography.bodyLarge,
            modifier = Modifier.announced(),
        )
        if (message == CatalogMessage.Conflict && onReload != null) {
            OutlinedButton(onClick = onReload, modifier = Modifier.heightIn(min = TouchTarget)) {
                Text(stringResource(Res.string.message_reload))
            }
        }
    }
}

/**
 * OfflineBanner says the server did not answer and what is shown is the device's copy: the shared connection
 * message, then when the copy was saved.
 */
@Composable
internal fun OfflineBanner(
    savedAtEpochMillis: Long,
    modifier: Modifier = Modifier,
) {
    Column(modifier = modifier.fillMaxWidth().announced(), verticalArrangement = Arrangement.spacedBy(4.dp)) {
        Text(
            text = FailureMessage.Offline.text(),
            style = MaterialTheme.typography.bodyLarge,
            color = MaterialTheme.colorScheme.error,
        )
        Text(
            text = offlineSavedText(savedAtEpochMillis),
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

/** A text field at the workshop size, with its problem under it. */
@Composable
internal fun CatalogTextField(
    label: String,
    value: String,
    onValueChange: (String) -> Unit,
    modifier: Modifier = Modifier,
    problem: String? = null,
    hint: String? = null,
    keyboardType: KeyboardType = KeyboardType.Text,
    singleLine: Boolean = true,
) {
    // A problem is announced when it appears; a hint is just there.
    val supportingText: (@Composable () -> Unit)? =
        (problem ?: hint)?.let { text ->
            { Text(text, modifier = if (problem == null) Modifier else Modifier.announced()) }
        }
    OutlinedTextField(
        value = value,
        onValueChange = onValueChange,
        label = { Text(label) },
        singleLine = singleLine,
        isError = problem != null,
        supportingText = supportingText,
        keyboardOptions = KeyboardOptions(keyboardType = keyboardType),
        modifier = modifier.fillMaxWidth().heightIn(min = TouchTarget),
    )
}

/** One choice of a [ChoiceField]. */
internal data class Choice<T>(
    val value: T,
    val label: String,
)

/** A button that opens a menu of [choices], with [selected] shown, and a problem under it. */
@Composable
internal fun <T> ChoiceField(
    label: String,
    selectedLabel: String?,
    choices: List<Choice<T>>,
    onPick: (T) -> Unit,
    modifier: Modifier = Modifier,
    problem: String? = null,
    enabled: Boolean = true,
) {
    var open by remember { mutableStateOf(false) }
    // The label above names the choice; the button's own text is the answer, read out as its state.
    val buttonModifier =
        Modifier
            .fillMaxWidth()
            .heightIn(min = TouchTarget)
            .semantics {
                role = Role.DropdownList
                stateDescription = selectedLabel.orEmpty()
            }
    Column(modifier = modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(4.dp)) {
        Text(text = label, style = MaterialTheme.typography.labelLarge)
        Box {
            OutlinedButton(onClick = { open = true }, enabled = enabled, modifier = buttonModifier) {
                Text(selectedLabel.orEmpty())
            }
            DropdownMenu(expanded = open, onDismissRequest = { open = false }) {
                choices.forEach { choice ->
                    DropdownMenuItem(
                        text = { Text(choice.label, style = MaterialTheme.typography.bodyLarge) },
                        onClick = {
                            open = false
                            onPick(choice.value)
                        },
                        modifier = Modifier.heightIn(min = TouchTarget),
                    )
                }
            }
        }
        problem?.let {
            Text(
                text = it,
                color = MaterialTheme.colorScheme.error,
                style = MaterialTheme.typography.bodyMedium,
                modifier = Modifier.announced(),
            )
        }
    }
}

/** A full-width primary button, 64 dp tall. */
@Composable
internal fun PrimaryButton(
    text: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    enabled: Boolean = true,
) {
    Button(onClick = onClick, enabled = enabled, modifier = modifier.fillMaxWidth().heightIn(min = PrimaryTarget)) {
        Text(text)
    }
}

/** A secondary button, 56 dp tall. */
@Composable
internal fun SecondaryButton(
    text: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    enabled: Boolean = true,
) {
    OutlinedButton(
        onClick = onClick,
        enabled = enabled,
        modifier = modifier.heightIn(min = TouchTarget),
    ) { Text(text) }
}

/** A section heading with a little room above. */
@Composable
internal fun SectionTitle(text: String) {
    Text(
        text = text,
        style = MaterialTheme.typography.titleLarge,
        modifier = Modifier.padding(top = 8.dp),
    )
}
