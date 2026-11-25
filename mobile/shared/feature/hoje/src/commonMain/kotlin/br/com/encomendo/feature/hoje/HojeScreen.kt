package br.com.encomendo.feature.hoje

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import br.com.encomendo.feature.hoje.resources.Res
import br.com.encomendo.feature.hoje.resources.hoje_placeholder
import br.com.encomendo.feature.hoje.resources.hoje_title
import org.jetbrains.compose.resources.stringResource

/**
 * HojeScreen is the management home: what needs the owner today. For now it
 * is a placeholder; the summary cards, orders by delivery date and "Precisa
 * de você" alerts arrive with orders in milestone 4, with a HojeViewModel.
 */
@Composable
fun HojeScreen(modifier: Modifier = Modifier) {
    Column(
        modifier = modifier.fillMaxSize().padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        Text(text = stringResource(Res.string.hoje_title), style = MaterialTheme.typography.headlineSmall)
        Text(text = stringResource(Res.string.hoje_placeholder), style = MaterialTheme.typography.bodyLarge)
    }
}
