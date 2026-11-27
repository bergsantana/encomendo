package br.com.encomendo.feature.catalogo

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import br.com.encomendo.core.domain.catalog.CategoryRepository
import br.com.encomendo.core.domain.catalog.ItemFilter
import br.com.encomendo.core.domain.catalog.ItemRepository
import br.com.encomendo.core.domain.catalog.ItemSummary
import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.domain.session.ObserveSession
import br.com.encomendo.core.domain.session.Permission
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/**
 * ItemsViewModel backs the Itens list of Catálogo, as the web's items store: search, the category filter,
 * "Mostrar arquivados", "Carregar mais" and the "No portal" switch (docs/product/design-document.md, "Catálogo").
 * It reads a page at a time through the server first and says when the rows are the device's copy. A change made
 * on a screen opened from here (the item form, Categorias, Campos) arrives through [CatalogChanges] and is read
 * again here.
 */
class ItemsViewModel(
    observeSession: ObserveSession,
    private val items: ItemRepository,
    private val categories: CategoryRepository,
    changes: CatalogChanges,
) : ViewModel() {
    private val list = MutableStateFlow(ItemsUiState())
    private var cachedAt: Long? = null
    private var searching: Job? = null

    private val pager =
        Pager(viewModelScope) { cursor ->
            val fetched = items.itemsPage(currentFilter(), cursor, PAGE_SIZE)
            cachedAt = fetched.cachedAtEpochMillis
            fetched.value
        }

    private val firstPage =
        PageReader<ItemSummary>(
            onStart = { list.update { it.copy(loading = true, loadingMore = false, message = null) } },
            onPage = { page ->
                list.update {
                    it.copy(items = page.items, hasMore = page.nextCursor != null, offlineSince = cachedAt)
                }
            },
            onFailure = { failure -> list.update { it.copy(message = failure.toCatalogMessage()) } },
            onEnd = { list.update { it.copy(loading = false) } },
        )

    private val nextPage =
        PageReader<ItemSummary>(
            onStart = { list.update { it.copy(loadingMore = true, message = null) } },
            onPage = { page ->
                list.update { it.copy(items = it.items + page.items, hasMore = page.nextCursor != null) }
            },
            onFailure = { failure -> list.update { it.copy(message = failure.toCatalogMessage()) } },
            onEnd = { list.update { it.copy(loadingMore = false) } },
        )

    /** The list as it shows now. */
    val uiState: StateFlow<ItemsUiState> = list.asStateFlow()

    init {
        viewModelScope.launch {
            observeSession().collect { session ->
                val canWrite = session?.permissions?.allows(Permission.CatalogWrite) == true
                list.update { it.copy(canWrite = canWrite) }
            }
        }
        viewModelScope.launch {
            changes.changes.collect { change -> onChanged(change) }
        }
        readCategories()
        pager.readFirst(firstPage)
    }

    /** "Tentar de novo" after the list could not be read. */
    fun onRetry() {
        readCategories()
        pager.readFirst(firstPage)
    }

    /** The text in "Buscar item" changed; the list reads again once typing pauses. */
    fun onQueryChange(query: String) {
        list.update { it.copy(query = query) }
        searching?.cancel()
        searching =
            viewModelScope.launch {
                delay(SEARCH_PAUSE_MILLIS)
                pager.readFirst(firstPage)
            }
    }

    /** The category filter changed; null is "Todas as categorias". */
    fun onCategoryChange(categoryId: String?) {
        list.update { it.copy(categoryId = categoryId) }
        searchNow()
    }

    /** "Mostrar arquivados" changed. */
    fun onIncludeArchivedChange(include: Boolean) {
        list.update { it.copy(includeArchived = include) }
        searchNow()
    }

    /** "Carregar mais": the next page. Problems show at the top. */
    fun onLoadMore() {
        pager.readNext(nextPage)
    }

    /** The "No portal" switch of [item]: shows or hides it on the store page. One change at a time. */
    fun onTogglePublished(
        item: ItemSummary,
        published: Boolean,
    ) {
        val state = list.value
        if (!state.canWrite || state.savingId != null || item.archived) return
        list.update { it.copy(savingId = item.id, message = null, notice = null) }
        viewModelScope.launch {
            try {
                val saved = items.setItemPublished(item.id, published, item.version)
                list.update { state ->
                    val row = { old: ItemSummary -> old.copy(published = saved.published, version = saved.version) }
                    state.copy(items = state.items.map { if (it.id == item.id) row(it) else it })
                }
            } catch (failure: Failure) {
                val message = failure.toCatalogMessage()
                list.update { it.copy(message = message) }
                if (message == CatalogMessage.Conflict) {
                    // Someone else saved first, so the list is stale. Reading clears the message: say it again.
                    pager.readFirst(firstPage)
                    list.update { it.copy(message = message) }
                }
            } finally {
                list.update { it.copy(savingId = null) }
            }
        }
    }

    private fun searchNow() {
        searching?.cancel()
        pager.readFirst(firstPage)
    }

    private fun onChanged(change: CatalogChange) {
        when (change) {
            CatalogChange.Taxonomy -> {
                list.update { it.copy(notice = null) }
                readCategories()
            }
            else -> list.update { it.copy(notice = change) }
        }
        pager.readFirst(firstPage)
    }

    private fun readCategories() {
        viewModelScope.launch {
            try {
                val read = categories.categories().value
                val before = list.value.categoryId
                // A category deleted meanwhile cannot stay the filter, and the rows were read with it.
                val kept = before?.takeIf { id -> read.any { it.id == id } }
                list.update { it.copy(categories = read, categoryId = kept) }
                if (kept != before) pager.readFirst(firstPage)
            } catch (_: Failure) {
                // The filter just stays as it was: the list's own read says what went wrong.
            }
        }
    }

    private fun currentFilter(): ItemFilter {
        val state = list.value
        return ItemFilter(
            query = state.query.trim().ifEmpty { null },
            categoryId = state.categoryId,
            includeArchived = state.includeArchived,
        )
    }

    private companion object {
        const val PAGE_SIZE = 20
        const val SEARCH_PAUSE_MILLIS = 300L
    }
}
