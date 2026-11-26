package br.com.encomendo.core.navigation

import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalDrawerSheet
import androidx.compose.material3.NavigationDrawerItem
import androidx.compose.material3.NavigationDrawerItemDefaults
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.paneTitle
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.navigation.NavDestination
import br.com.encomendo.core.navigation.resources.Res
import br.com.encomendo.core.navigation.resources.menu_brand
import br.com.encomendo.core.navigation.resources.menu_open
import br.com.encomendo.core.navigation.resources.menu_title
import br.com.encomendo.core.ui.LocalGrantedPermissions
import org.jetbrains.compose.resources.stringResource

/**
 * ManagementMenu is the drawer of the management screens: the entries the
 * session's permissions allow, with [current] marked. [onOpen] gets the
 * chosen entry's destination.
 */
@Composable
internal fun ManagementMenu(
    current: NavDestination?,
    onOpen: (Any) -> Unit,
) {
    val title = stringResource(Res.string.menu_title)
    ModalDrawerSheet(modifier = Modifier.semantics { paneTitle = title }) {
        Text(
            text = stringResource(Res.string.menu_brand),
            style = MaterialTheme.typography.titleLarge,
            modifier = Modifier.padding(horizontal = 28.dp, vertical = 16.dp),
        )
        MenuEntry.allowedBy(LocalGrantedPermissions.current).forEach { entry ->
            NavigationDrawerItem(
                label = { Text(stringResource(entry.label)) },
                selected = entry.isAt(current),
                onClick = { onOpen(entry.destination) },
                modifier = Modifier.padding(NavigationDrawerItemDefaults.ItemPadding),
            )
        }
    }
}

/** MenuButton opens the management menu; it sits before the store's name on the account bar. */
@Composable
internal fun MenuButton(onClick: () -> Unit) {
    // The account bar tops the workshop screens too, so its targets follow their 56 dp rule (mobile/CLAUDE.md).
    TextButton(onClick = onClick, modifier = Modifier.heightIn(min = 56.dp)) {
        Text(stringResource(Res.string.menu_open))
    }
}
