package br.com.encomendo.feature.catalogo

import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.asSharedFlow

/** CatalogChange is something a Catálogo screen changed that the list behind it must show. */
sealed interface CatalogChange {
    /** "Item salvo." */
    data object ItemSaved : CatalogChange, ItemNotice

    /** "{item} foi arquivado." */
    data class ItemArchived(
        val name: String,
    ) : CatalogChange,
        ItemNotice

    /** "{item} voltou ao catálogo." */
    data class ItemRestored(
        val name: String,
    ) : CatalogChange,
        ItemNotice

    /** A category or a field changed: the list's filter and its rows' categories are read again. */
    data object Taxonomy : CatalogChange
}

/** ItemNotice is a [CatalogChange] that the list says in words, as a status line. */
sealed interface ItemNotice

/**
 * CatalogChanges tells the Itens list what the screens opened from it changed, so it shows the notice and reads
 * again without the screens knowing each other. Nothing is kept: a list that is not alive reads fresh when it
 * starts, so a change is never replayed to a later store's screens.
 */
class CatalogChanges {
    private val flow = MutableSharedFlow<CatalogChange>(extraBufferCapacity = BUFFER)

    /** The changes made from now on. */
    val changes: Flow<CatalogChange> = flow.asSharedFlow()

    /** Tells whoever listens that [change] was made. */
    fun publish(change: CatalogChange) {
        flow.tryEmit(change)
    }

    private companion object {
        const val BUFFER = 16
    }
}
