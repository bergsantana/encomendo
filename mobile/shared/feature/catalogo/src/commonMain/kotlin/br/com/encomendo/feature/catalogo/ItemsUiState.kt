package br.com.encomendo.feature.catalogo

import br.com.encomendo.core.domain.catalog.Category
import br.com.encomendo.core.domain.catalog.ItemSummary
import br.com.encomendo.core.domain.catalog.isShown

/**
 * ItemsUiState is the Itens list: the rows read so far, the filters, whether the rows are the device's copy, and
 * what the last change said. Rows carry cost and margin only when the API sent them, so a screen shows those
 * columns exactly when [showsCosts] is true.
 */
data class ItemsUiState(
    /** The rows read so far, by name. */
    val items: List<ItemSummary> = emptyList(),
    /** The store's categories, for the filter and each row's "Categoria". */
    val categories: List<Category> = emptyList(),
    /** The text in "Buscar item". */
    val query: String = "",
    /** The category the list is filtered to, or null for "Todas as categorias". */
    val categoryId: String? = null,
    /** "Mostrar arquivados". */
    val includeArchived: Boolean = false,
    /** The first page is being read: "Carregando o catálogo…" while no row shows. */
    val loading: Boolean = true,
    /** The list runs past what was read: "Carregar mais". */
    val hasMore: Boolean = false,
    /** The next page is being read. */
    val loadingMore: Boolean = false,
    /** When the rows shown were saved on the device, in epoch milliseconds, if the server did not answer. */
    val offlineSince: Long? = null,
    /** The role may change the catalog (`catalog:write`): the buttons and the "No portal" switch show. */
    val canWrite: Boolean = false,
    /** The row whose "No portal" switch is being saved. */
    val savingId: String? = null,
    /** The last change that went through, for the status line. */
    val notice: CatalogChange? = null,
    /** Why the last read or change failed. */
    val message: CatalogMessage? = null,
) {
    /** True when the API sent a cost or a margin on a row: the owner's columns show. */
    val showsCosts: Boolean get() = items.any { it.cost.isShown || it.margin.isShown }

    /** True while a filter or search is set, so an empty list says "Nenhum item encontrado." */
    val isFiltered: Boolean get() = query.isNotBlank() || categoryId != null || includeArchived

    /** The read ended with nothing to show and no failure. */
    val isEmpty: Boolean get() = !loading && items.isEmpty() && message == null

    /** The first read failed, so "Tentar de novo" shows. */
    val canRetry: Boolean get() = message != null && items.isEmpty() && !loading

    /** The name of the category [id], or null for "Sem categoria". */
    fun categoryName(id: String?): String? = categories.firstOrNull { it.id == id }?.name
}
