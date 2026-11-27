package br.com.encomendo.feature.catalogo

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import br.com.encomendo.core.domain.catalog.KitFilter
import br.com.encomendo.core.domain.catalog.KitSummary
import br.com.encomendo.core.domain.catalog.ListKits
import br.com.encomendo.core.domain.catalog.SetKitPublished
import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.domain.paging.PageReader
import br.com.encomendo.core.domain.paging.Pager
import br.com.encomendo.core.domain.session.ObserveSession
import br.com.encomendo.core.domain.session.Permission
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/**
 * KitsViewModel backs the Kits tab of Catálogo, as the web's kits store: "Mostrar arquivados", "Carregar mais" and
 * the "No portal" switch (docs/product/design-document.md, "Catálogo", "Kits"). It reads a page at a time through the
 * server first and says when the rows are the device's copy. A change made from a screen opened from here (the kit
 * editor) or on an item (its price is in the kit's "Soma avulsa") arrives through [CatalogChanges] and is read again.
 */
class KitsViewModel(
    observeSession: ObserveSession,
    private val listKits: ListKits,
    private val setKitPublished: SetKitPublished,
    changes: CatalogChanges,
) : ViewModel() {
    private val list = MutableStateFlow(KitsUiState())
    private var cachedAt: Long? = null

    private val pager =
        Pager(viewModelScope) { cursor ->
            val fetched = listKits(KitFilter(includeArchived = list.value.includeArchived), cursor, PAGE_SIZE)
            cachedAt = fetched.cachedAtEpochMillis
            fetched.value
        }

    private val firstPage =
        PageReader<KitSummary>(
            onStart = { list.update { it.copy(loading = true, loadingMore = false, message = null) } },
            onPage = { page ->
                list.update { it.copy(kits = page.items, hasMore = page.nextCursor != null, offlineSince = cachedAt) }
            },
            onFailure = { failure -> list.update { it.copy(message = failure.toCatalogMessage()) } },
            onEnd = { list.update { it.copy(loading = false) } },
        )

    private val nextPage =
        PageReader<KitSummary>(
            onStart = { list.update { it.copy(loadingMore = true, message = null) } },
            onPage = { page ->
                list.update { it.copy(kits = it.kits + page.items, hasMore = page.nextCursor != null) }
            },
            onFailure = { failure -> list.update { it.copy(message = failure.toCatalogMessage()) } },
            onEnd = { list.update { it.copy(loadingMore = false) } },
        )

    /** The list as it shows now. */
    val uiState: StateFlow<KitsUiState> = list.asStateFlow()

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
        pager.readFirst(firstPage)
    }

    /** "Tentar de novo" after the list could not be read. */
    fun onRetry() = pager.readFirst(firstPage)

    /** "Mostrar arquivados" changed. */
    fun onIncludeArchivedChange(include: Boolean) {
        list.update { it.copy(includeArchived = include) }
        pager.readFirst(firstPage)
    }

    /** "Carregar mais": the next page. Problems show at the top. */
    fun onLoadMore() {
        pager.readNext(nextPage)
    }

    /** The "No portal" switch of [kit]: shows or hides it on the store page. One change at a time. */
    fun onTogglePublished(
        kit: KitSummary,
        published: Boolean,
    ) {
        val state = list.value
        if (!state.canWrite || state.savingId != null || kit.archived) return
        list.update { it.copy(savingId = kit.id, message = null, notice = null) }
        viewModelScope.launch {
            try {
                val saved = setKitPublished(kit.id, published, kit.version)
                list.update { state ->
                    state.copy(
                        kits =
                            state.kits.map {
                                if (it.id ==
                                    kit.id
                                ) {
                                    it.copy(published = saved.published, version = saved.version)
                                } else {
                                    it
                                }
                            },
                    )
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

    private fun onChanged(change: CatalogChange) {
        when (change) {
            is KitNotice -> list.update { it.copy(notice = change) }
            // The kit's "Soma avulsa" and "Prazo" follow its items, so an item's change reads the list again.
            CatalogChange.KitSaved, is ItemNotice -> list.update { it.copy(notice = null) }
            CatalogChange.Taxonomy -> return
        }
        pager.readFirst(firstPage)
    }

    private companion object {
        const val PAGE_SIZE = 20
    }
}
