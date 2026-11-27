package br.com.encomendo.feature.catalogo

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.selection.toggleable
import androidx.compose.material3.Checkbox
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import br.com.encomendo.core.domain.catalog.ItemSummary
import br.com.encomendo.feature.catalogo.resources.Res
import br.com.encomendo.feature.catalogo.resources.catalogo_title
import br.com.encomendo.feature.catalogo.resources.items_all_categories
import br.com.encomendo.feature.catalogo.resources.items_attributes
import br.com.encomendo.feature.catalogo.resources.items_categories
import br.com.encomendo.feature.catalogo.resources.items_empty
import br.com.encomendo.feature.catalogo.resources.items_load_more
import br.com.encomendo.feature.catalogo.resources.items_loading
import br.com.encomendo.feature.catalogo.resources.items_new
import br.com.encomendo.feature.catalogo.resources.items_none_found
import br.com.encomendo.feature.catalogo.resources.items_retry
import br.com.encomendo.feature.catalogo.resources.items_search
import br.com.encomendo.feature.catalogo.resources.items_show_archived
import org.jetbrains.compose.resources.stringResource
import org.koin.compose.viewmodel.koinViewModel

/** What the Itens list opens: an item (null for "Novo item"), "Categorias" or "Campos". */
data class ItemsNavigation(
    val onOpenItem: (itemId: String?) -> Unit,
    val onOpenCategories: () -> Unit,
    val onOpenAttributes: () -> Unit,
)

/** ItemsRoute connects [ItemsViewModel] to [ItemsScreen]. */
@Composable
fun ItemsRoute(
    navigation: ItemsNavigation,
    modifier: Modifier = Modifier,
    viewModel: ItemsViewModel = koinViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val actions =
        remember(viewModel, navigation) {
            ItemsActions(
                onQueryChange = viewModel::onQueryChange,
                onCategoryChange = viewModel::onCategoryChange,
                onIncludeArchivedChange = viewModel::onIncludeArchivedChange,
                onLoadMore = viewModel::onLoadMore,
                onRetry = viewModel::onRetry,
                onTogglePublished = viewModel::onTogglePublished,
                navigation = navigation,
            )
        }
    CatalogoTheme { ItemsScreen(state = state, actions = actions, modifier = modifier) }
}

/** ItemsActions are what the user can do on [ItemsScreen]. */
data class ItemsActions(
    val onQueryChange: (String) -> Unit,
    val onCategoryChange: (String?) -> Unit,
    val onIncludeArchivedChange: (Boolean) -> Unit,
    val onLoadMore: () -> Unit,
    val onRetry: () -> Unit,
    val onTogglePublished: (ItemSummary, Boolean) -> Unit,
    val navigation: ItemsNavigation,
)

/**
 * ItemsScreen is the Itens list of Catálogo (docs/product/design-document.md, "Catálogo"): the web's table as one
 * card per item, with search, a category filter, "Mostrar arquivados" and "Carregar mais". The buttons that change
 * anything show only to roles with `catalog:write`, and cost and margin only when the API sent them.
 */
@Composable
fun ItemsScreen(
    state: ItemsUiState,
    actions: ItemsActions,
    modifier: Modifier = Modifier,
) {
    LazyColumn(
        modifier = modifier.fillMaxSize(),
        contentPadding = PaddingValues(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        item(key = "header") { ItemsHeader(state = state, actions = actions) }
        items(state.items, key = { it.id }) { summary ->
            ItemCard(summary = summary, state = state, actions = actions)
        }
        if (state.hasMore) {
            item(key = "more") {
                SecondaryButton(
                    text = stringResource(Res.string.items_load_more),
                    onClick = actions.onLoadMore,
                    enabled = !state.loadingMore,
                )
            }
        }
    }
}

/** The title, the buttons, the filters and everything that says how the read went. */
@Composable
private fun ItemsHeader(
    state: ItemsUiState,
    actions: ItemsActions,
) {
    Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Text(
            text = stringResource(Res.string.catalogo_title),
            style = MaterialTheme.typography.headlineSmall,
            modifier = Modifier.semantics { heading() },
        )
        if (state.canWrite) {
            PrimaryButton(
                text = stringResource(Res.string.items_new),
                onClick = { actions.navigation.onOpenItem(null) },
            )
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                SecondaryButton(stringResource(Res.string.items_categories), actions.navigation.onOpenCategories)
                SecondaryButton(stringResource(Res.string.items_attributes), actions.navigation.onOpenAttributes)
            }
        }
        state.offlineSince?.let { OfflineBanner(savedAtEpochMillis = it) }
        state.notice?.let {
            Text(text = it.noticeText(), style = MaterialTheme.typography.bodyLarge, modifier = Modifier.announced())
        }
        state.message?.let { MessageText(message = it, onReload = { actions.onRetry() }) }
        Filters(state = state, actions = actions)
        ListStatus(state = state, onRetry = actions.onRetry)
    }
}

@Composable
private fun Filters(
    state: ItemsUiState,
    actions: ItemsActions,
) {
    CatalogTextField(
        label = stringResource(Res.string.items_search),
        value = state.query,
        onValueChange = actions.onQueryChange,
    )
    val all = stringResource(Res.string.items_all_categories)
    ChoiceField(
        label = all,
        selectedLabel = state.categoryName(state.categoryId) ?: all,
        choices = listOf(Choice<String?>(null, all)) + state.categories.map { Choice(it.id, it.name) },
        onPick = actions.onCategoryChange,
    )
    Row(
        modifier =
            Modifier
                .fillMaxWidth()
                .heightIn(min = TOUCH_TARGET)
                .toggleable(
                    value = state.includeArchived,
                    role = Role.Checkbox,
                    onValueChange = actions.onIncludeArchivedChange,
                ),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Checkbox(checked = state.includeArchived, onCheckedChange = null)
        Text(
            text = stringResource(Res.string.items_show_archived),
            style = MaterialTheme.typography.bodyLarge,
            modifier = Modifier.padding(start = 12.dp),
        )
    }
}

/** "Carregando o catálogo…", the empty states, and "Tentar de novo" after a failed first read. */
@Composable
private fun ListStatus(
    state: ItemsUiState,
    onRetry: () -> Unit,
) {
    when {
        state.loading && state.items.isEmpty() ->
            Text(
                text = stringResource(Res.string.items_loading),
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                style = MaterialTheme.typography.bodyLarge,
            )
        state.isEmpty ->
            Text(
                text = stringResource(if (state.isFiltered) Res.string.items_none_found else Res.string.items_empty),
                style = MaterialTheme.typography.bodyLarge,
            )
        state.canRetry -> SecondaryButton(stringResource(Res.string.items_retry), onRetry)
    }
}
