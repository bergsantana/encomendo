package br.com.encomendo.feature.catalogo

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.selection.toggleable
import androidx.compose.material3.Checkbox
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedCard
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import br.com.encomendo.core.domain.catalog.KitSummary
import br.com.encomendo.core.ui.PhotoImage
import br.com.encomendo.core.ui.TouchTarget
import br.com.encomendo.feature.catalogo.resources.Res
import br.com.encomendo.feature.catalogo.resources.col_kit_items
import br.com.encomendo.feature.catalogo.resources.col_kit_price
import br.com.encomendo.feature.catalogo.resources.col_kit_single_total
import br.com.encomendo.feature.catalogo.resources.col_lead
import br.com.encomendo.feature.catalogo.resources.col_margin
import br.com.encomendo.feature.catalogo.resources.items_archived
import br.com.encomendo.feature.catalogo.resources.items_load_more
import br.com.encomendo.feature.catalogo.resources.items_retry
import br.com.encomendo.feature.catalogo.resources.items_show_archived
import br.com.encomendo.feature.catalogo.resources.kits_empty
import br.com.encomendo.feature.catalogo.resources.kits_loading
import br.com.encomendo.feature.catalogo.resources.kits_new
import br.com.encomendo.feature.catalogo.resources.kits_open
import org.jetbrains.compose.resources.stringResource
import org.koin.compose.viewmodel.koinViewModel

/** KitsRoute connects [KitsViewModel] to [KitsScreen]; [onOpenKit] opens a kit, or "Novo kit" with null. */
@Composable
fun KitsRoute(
    onOpenKit: (kitId: String?) -> Unit,
    modifier: Modifier = Modifier,
    viewModel: KitsViewModel = koinViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val actions =
        remember(viewModel, onOpenKit) {
            KitsActions(
                onIncludeArchivedChange = viewModel::onIncludeArchivedChange,
                onLoadMore = viewModel::onLoadMore,
                onRetry = viewModel::onRetry,
                onTogglePublished = viewModel::onTogglePublished,
                onOpenKit = onOpenKit,
            )
        }
    CatalogoTheme { KitsScreen(state = state, actions = actions, modifier = modifier) }
}

/** KitsActions are what the user can do on [KitsScreen]. */
data class KitsActions(
    val onIncludeArchivedChange: (Boolean) -> Unit,
    val onLoadMore: () -> Unit,
    val onRetry: () -> Unit,
    val onTogglePublished: (KitSummary, Boolean) -> Unit,
    val onOpenKit: (kitId: String?) -> Unit,
)

/**
 * KitsScreen is the Kits tab of Catálogo (docs/product/design-document.md, "Catálogo", "Kits"): the web's table as
 * one card per kit, with "Mostrar arquivados" and "Carregar mais". "Novo kit" and the "No portal" switch work only for
 * roles with `catalog:write`, and "Margem" shows only when the API sent it. Everything shown is as the API sent it.
 */
@Composable
fun KitsScreen(
    state: KitsUiState,
    actions: KitsActions,
    modifier: Modifier = Modifier,
) {
    LazyColumn(
        modifier = modifier.fillMaxSize(),
        contentPadding = PaddingValues(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        item(key = "header") { KitsHeader(state = state, actions = actions) }
        items(state.kits, key = { it.id }) { summary -> KitCard(summary, state, actions) }
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

/** The buttons, "Mostrar arquivados" and everything that says how the read went. */
@Composable
private fun KitsHeader(
    state: KitsUiState,
    actions: KitsActions,
) {
    Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
        // On an empty list only this toolbar button is the primary one; the one by the empty state is plain.
        if (state.canWrite) PrimaryButton(stringResource(Res.string.kits_new), { actions.onOpenKit(null) })
        state.offlineSince?.let { OfflineBanner(savedAtEpochMillis = it) }
        state.notice?.let {
            Text(text = it.noticeText(), style = MaterialTheme.typography.bodyLarge, modifier = Modifier.announced())
        }
        state.message?.let { MessageText(message = it, onReload = actions.onRetry) }
        ShowArchived(checked = state.includeArchived, onChange = actions.onIncludeArchivedChange)
        KitsStatus(state = state, actions = actions)
    }
}

@Composable
private fun ShowArchived(
    checked: Boolean,
    onChange: (Boolean) -> Unit,
) {
    Row(
        modifier =
            Modifier
                .fillMaxWidth()
                .heightIn(min = TouchTarget)
                .toggleable(value = checked, role = Role.Checkbox, onValueChange = onChange),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Checkbox(checked = checked, onCheckedChange = null)
        Text(
            text = stringResource(Res.string.items_show_archived),
            style = MaterialTheme.typography.bodyLarge,
            modifier = Modifier.padding(start = 12.dp),
        )
    }
}

/** "Carregando os kits…", "Nenhum kit ainda." and "Tentar de novo" after a failed first read. */
@Composable
private fun KitsStatus(
    state: KitsUiState,
    actions: KitsActions,
) {
    when {
        state.loading && state.kits.isEmpty() ->
            Text(
                text = stringResource(Res.string.kits_loading),
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                style = MaterialTheme.typography.bodyLarge,
            )
        state.isEmpty -> {
            Text(text = stringResource(Res.string.kits_empty), style = MaterialTheme.typography.bodyLarge)
            if (state.canWrite) SecondaryButton(stringResource(Res.string.kits_new), { actions.onOpenKit(null) })
        }
        state.canRetry -> SecondaryButton(stringResource(Res.string.items_retry), actions.onRetry)
    }
}

/**
 * KitCard is one row of the web's table on a phone: name, "Arquivado", Itens, Preço do kit, Soma avulsa, Margem only
 * when the API sent it, Prazo, and the "No portal" switch. Tapping it opens the kit.
 */
@Composable
internal fun KitCard(
    summary: KitSummary,
    state: KitsUiState,
    actions: KitsActions,
) {
    val open = stringResource(Res.string.kits_open, summary.name)
    OutlinedCard(
        modifier = Modifier.fillMaxWidth().clickable(onClickLabel = open) { actions.onOpenKit(summary.id) },
    ) {
        Column(modifier = Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            summary.cover?.let { cover ->
                PhotoImage(
                    url = cover.url,
                    contentDescription = null,
                    modifier = Modifier.size(COVER_SIZE).clip(MaterialTheme.shapes.small),
                )
            }
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.CenterVertically) {
                Text(
                    text = summary.name,
                    style = MaterialTheme.typography.titleMedium,
                    modifier = Modifier.weight(1f, fill = false),
                )
                if (summary.archived) {
                    Text(
                        text = stringResource(Res.string.items_archived),
                        style = MaterialTheme.typography.labelLarge,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
            ItemLine(stringResource(Res.string.col_kit_items), summary.itemCount.toString())
            ItemLine(stringResource(Res.string.col_kit_price), formatMoney(summary.price))
            ItemLine(stringResource(Res.string.col_kit_single_total), formatMoney(summary.singleItemsTotal))
            summary.margin.text(summary.price.currency)?.let { ItemLine(stringResource(Res.string.col_margin), it) }
            ItemLine(
                stringResource(Res.string.col_lead),
                daysLabel(summary.totalDays),
            )
            PortalSwitchRow(
                name = summary.name,
                published = summary.published,
                enabled = state.canWrite && !summary.archived && state.savingId == null,
                onToggle = { actions.onTogglePublished(summary, it) },
            )
        }
    }
}

private val COVER_SIZE = 72.dp
