package br.com.encomendo.feature.auth

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import br.com.encomendo.feature.auth.resources.Res
import br.com.encomendo.feature.auth.resources.login_placeholder
import br.com.encomendo.feature.auth.resources.login_title
import org.jetbrains.compose.resources.stringResource

/**
 * LoginScreen is where a signed-out business user starts. For now it is a
 * placeholder: the form, sign-up, tenant picker and forgot password arrive
 * with the /auth contract (docs/parallel-sessions.md, Mobile session).
 */
@Composable
fun LoginScreen(modifier: Modifier = Modifier) {
    Column(
        modifier = modifier.fillMaxSize().padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        Text(text = stringResource(Res.string.login_title), style = MaterialTheme.typography.headlineSmall)
        Text(text = stringResource(Res.string.login_placeholder), style = MaterialTheme.typography.bodyLarge)
    }
}
