package br.com.encomendo.feature.auth

import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.size
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp

/**
 * BusyButton is a form's action: [label] while idle, a spinner read out as
 * [busyLabel] while [busy]. Full width and 56 dp high, so it is easy to hit;
 * filled when [primary], outlined beside another main action.
 */
@Composable
internal fun BusyButton(
    label: String,
    busyLabel: String,
    busy: Boolean,
    enabled: Boolean,
    onClick: () -> Unit,
    primary: Boolean = true,
) {
    val modifier =
        Modifier
            .fillMaxWidth()
            .heightIn(min = 56.dp)
            .semantics { if (busy) contentDescription = busyLabel }
    val content: @Composable RowScope.() -> Unit = {
        if (busy) {
            CircularProgressIndicator(modifier = Modifier.size(24.dp), strokeWidth = 2.dp)
        } else {
            Text(label)
        }
    }
    if (primary) {
        Button(onClick = onClick, enabled = enabled, modifier = modifier, content = content)
    } else {
        OutlinedButton(onClick = onClick, enabled = enabled, modifier = modifier, content = content)
    }
}
