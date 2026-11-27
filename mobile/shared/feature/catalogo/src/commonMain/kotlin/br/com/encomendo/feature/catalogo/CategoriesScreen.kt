package br.com.encomendo.feature.catalogo

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedCard
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import br.com.encomendo.core.domain.catalog.Category
import br.com.encomendo.feature.catalogo.resources.Res
import br.com.encomendo.feature.catalogo.resources.categories_loading
import br.com.encomendo.feature.catalogo.resources.categories_title
import br.com.encomendo.feature.catalogo.resources.category_cancel
import br.com.encomendo.feature.catalogo.resources.category_delete
import br.com.encomendo.feature.catalogo.resources.category_move_down
import br.com.encomendo.feature.catalogo.resources.category_move_up
import br.com.encomendo.feature.catalogo.resources.category_name
import br.com.encomendo.feature.catalogo.resources.category_new
import br.com.encomendo.feature.catalogo.resources.category_rename
import br.com.encomendo.feature.catalogo.resources.category_save
import br.com.encomendo.feature.catalogo.resources.items_retry
import org.jetbrains.compose.resources.stringResource
import org.koin.compose.viewmodel.koinViewModel

/** CategoriesRoute connects [CategoriesViewModel] to [CategoriesScreen]. */
@Composable
fun CategoriesRoute(
    modifier: Modifier = Modifier,
    viewModel: CategoriesViewModel = koinViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val actions =
        remember(viewModel) {
            CategoriesActions(
                onNewNameChange = viewModel::onNewNameChange,
                onAdd = viewModel::onAdd,
                onStartRename = viewModel::onStartRename,
                onRenameChange = viewModel::onRenameChange,
                onSaveRename = viewModel::onSaveRename,
                onCancelRename = viewModel::onCancelRename,
                onMove = viewModel::onMove,
                onDelete = viewModel::onDelete,
                onRetry = viewModel::onRetry,
            )
        }
    CatalogoTheme { CategoriesScreen(state = state, actions = actions, modifier = modifier) }
}

/** CategoriesActions are what the user can do on [CategoriesScreen]. */
data class CategoriesActions(
    val onNewNameChange: (String) -> Unit,
    val onAdd: () -> Unit,
    val onStartRename: (Category) -> Unit,
    val onRenameChange: (String) -> Unit,
    val onSaveRename: () -> Unit,
    val onCancelRename: () -> Unit,
    val onMove: (Category, Boolean) -> Unit,
    val onDelete: (Category) -> Unit,
    val onRetry: () -> Unit,
)

/**
 * CategoriesScreen is "Categorias" (docs/product/design-document.md, "Categorias"): the store's categories in
 * order, with "Mover para cima", "Mover para baixo", "Renomear" and "Excluir" on each, and "Nova categoria". A
 * role that cannot write sees the list only.
 */
@Composable
fun CategoriesScreen(
    state: CategoriesUiState,
    actions: CategoriesActions,
    modifier: Modifier = Modifier,
) {
    LazyColumn(
        modifier = modifier.fillMaxSize().imePadding(),
        contentPadding = PaddingValues(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        item(key = "header") {
            Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                Text(
                    text = stringResource(Res.string.categories_title),
                    style = MaterialTheme.typography.headlineSmall,
                    modifier = Modifier.semantics { heading() },
                )
                state.offlineSince?.let { OfflineBanner(savedAtEpochMillis = it) }
                state.message?.let { MessageText(message = it) }
                if (state.loading && state.categories.isEmpty()) Text(stringResource(Res.string.categories_loading))
                if (state.message != null && state.categories.isEmpty() && !state.loading) {
                    SecondaryButton(stringResource(Res.string.items_retry), actions.onRetry)
                }
                if (state.canWrite) NewCategory(state, actions)
            }
        }
        items(state.categories, key = { it.id }) { category -> CategoryCard(category, state, actions) }
    }
}

@Composable
private fun NewCategory(
    state: CategoriesUiState,
    actions: CategoriesActions,
) {
    CatalogTextField(
        label = stringResource(Res.string.category_name),
        value = state.newName,
        onValueChange = actions.onNewNameChange,
        problem = state.newProblem?.categoryText(),
    )
    PrimaryButton(stringResource(Res.string.category_new), actions.onAdd, enabled = !state.busy)
}

@Composable
private fun CategoryCard(
    category: Category,
    state: CategoriesUiState,
    actions: CategoriesActions,
) {
    OutlinedCard(modifier = Modifier.fillMaxWidth()) {
        Column(modifier = Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            val edit = state.edit?.takeIf { it.categoryId == category.id }
            if (edit != null) {
                CategoryRename(edit, state, actions)
            } else {
                Text(text = category.name, style = MaterialTheme.typography.titleMedium)
                if (state.canWrite) CategoryButtons(category, state, actions)
            }
            if (state.inUseId == category.id) MessageText(CatalogMessage.CategoryInUse)
        }
    }
}

@Composable
private fun CategoryRename(
    edit: CategoryEdit,
    state: CategoriesUiState,
    actions: CategoriesActions,
) {
    CatalogTextField(
        label = stringResource(Res.string.category_name),
        value = edit.name,
        onValueChange = actions.onRenameChange,
        problem = edit.problem?.categoryText(),
    )
    FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
        SecondaryButton(stringResource(Res.string.category_save), actions.onSaveRename, enabled = !state.busy)
        SecondaryButton(stringResource(Res.string.category_cancel), actions.onCancelRename)
    }
}

/** The four changes to a row, each read out with the category's name. */
@Composable
private fun CategoryButtons(
    category: Category,
    state: CategoriesUiState,
    actions: CategoriesActions,
) {
    val first = state.categories.firstOrNull()?.id == category.id
    val last = state.categories.lastOrNull()?.id == category.id
    val of = { label: String -> Modifier.semantics { contentDescription = "$label: ${category.name}" } }
    val up = stringResource(Res.string.category_move_up)
    val down = stringResource(Res.string.category_move_down)
    val rename = stringResource(Res.string.category_rename)
    val delete = stringResource(Res.string.category_delete)
    FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
        SecondaryButton(up, { actions.onMove(category, true) }, of(up), enabled = !state.busy && !first)
        SecondaryButton(down, { actions.onMove(category, false) }, of(down), enabled = !state.busy && !last)
        SecondaryButton(rename, { actions.onStartRename(category) }, of(rename), enabled = !state.busy)
        SecondaryButton(delete, { actions.onDelete(category) }, of(delete), enabled = !state.busy)
    }
}
