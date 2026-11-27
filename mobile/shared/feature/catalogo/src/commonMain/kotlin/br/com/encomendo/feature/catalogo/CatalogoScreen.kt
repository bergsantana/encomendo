package br.com.encomendo.feature.catalogo

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.PrimaryTabRow
import androidx.compose.material3.Tab
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import br.com.encomendo.core.ui.TouchTarget
import br.com.encomendo.feature.catalogo.resources.Res
import br.com.encomendo.feature.catalogo.resources.catalogo_title
import br.com.encomendo.feature.catalogo.resources.tab_items
import br.com.encomendo.feature.catalogo.resources.tab_kits
import org.jetbrains.compose.resources.stringResource

/** What Catálogo opens: what the Itens tab opens, and a kit (null for "Novo kit"). */
data class CatalogoNavigation(
    val items: ItemsNavigation,
    val onOpenKit: (kitId: String?) -> Unit,
)

/** The tabs of Catálogo, in order. */
internal enum class CatalogoTab {
    /** The items. */
    Items,

    /** The kits. */
    Kits,
}

/**
 * CatalogoRoute is the Catálogo destination: the "Itens" and "Kits" tabs, each with its own list and ViewModel. The
 * open tab survives leaving for a form and coming back.
 */
@Composable
fun CatalogoRoute(
    navigation: CatalogoNavigation,
    modifier: Modifier = Modifier,
) {
    var selected by rememberSaveable { mutableIntStateOf(CatalogoTab.Items.ordinal) }
    CatalogoTheme {
        CatalogoScreen(
            selected = CatalogoTab.entries[selected],
            onSelect = { selected = it.ordinal },
            modifier = modifier,
        ) { tab ->
            when (tab) {
                CatalogoTab.Items -> ItemsRoute(navigation.items)
                CatalogoTab.Kits -> KitsRoute(navigation.onOpenKit)
            }
        }
    }
}

/**
 * CatalogoScreen is the title and the tab list of Catálogo over the open tab's [content]. Each tab is read out as a
 * tab with its selected state, and the tabs are 56 dp tall for gloves.
 */
@Composable
internal fun CatalogoScreen(
    selected: CatalogoTab,
    onSelect: (CatalogoTab) -> Unit,
    modifier: Modifier = Modifier,
    content: @Composable (CatalogoTab) -> Unit,
) {
    Column(modifier = modifier.fillMaxSize()) {
        Text(
            text = stringResource(Res.string.catalogo_title),
            style = MaterialTheme.typography.headlineSmall,
            modifier = Modifier.padding(start = 16.dp, top = 16.dp, end = 16.dp).semantics { heading() },
        )
        PrimaryTabRow(selectedTabIndex = selected.ordinal) {
            CatalogoTab.entries.forEach { tab ->
                Tab(
                    selected = tab == selected,
                    onClick = { onSelect(tab) },
                    modifier = Modifier.heightIn(min = TouchTarget),
                    text = { Text(stringResource(tab.label()), style = MaterialTheme.typography.bodyLarge) },
                )
            }
        }
        content(selected)
    }
}

private fun CatalogoTab.label() =
    when (this) {
        CatalogoTab.Items -> Res.string.tab_items
        CatalogoTab.Kits -> Res.string.tab_kits
    }
