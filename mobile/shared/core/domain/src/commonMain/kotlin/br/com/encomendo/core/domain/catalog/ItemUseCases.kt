package br.com.encomendo.core.domain.catalog

import br.com.encomendo.core.domain.paging.Page

// The item use cases are thin on purpose: the repository owns the rules (versions, costs, archiving), and the
// session-wide failures (a suspended store) are handled below it, in core:data's StoreCalls. Each exists so a
// ViewModel depends on one thing it does, as every feature does (mobile/CLAUDE.md, Layers).

/** ListItems reads a page of the Itens list. */
class ListItems(
    private val items: ItemRepository,
) {
    /** Returns up to [limit] items matching [filter] from [cursor] on; null [cursor] reads the first page. */
    suspend operator fun invoke(
        filter: ItemFilter,
        cursor: String?,
        limit: Int,
    ): Fetched<Page<ItemSummary>> = items.itemsPage(filter, cursor, limit)
}

/** GetItem reads one item for the item form. */
class GetItem(
    private val items: ItemRepository,
) {
    /** Returns the item [id]; throws `Failure.NotFound` when it is gone. */
    suspend operator fun invoke(id: String): Fetched<Item> = items.item(id)
}

/** SaveItem creates an item, or replaces one against the version the user read ("Salvar"). */
class SaveItem(
    private val items: ItemRepository,
) {
    /** Creates the item [draft] and returns it. */
    suspend fun create(draft: ItemDraft): Item = items.createItem(draft)

    /** Replaces the item [id] given the [version] read; throws `Failure.VersionConflict` if it moved on. */
    suspend fun replace(
        id: String,
        version: Int,
        draft: ItemDraft,
    ): Item = items.replaceItem(id, version, draft)
}

/** SetItemPublished flips "No portal" for an item. */
class SetItemPublished(
    private val items: ItemRepository,
) {
    /** Shows or hides the item [id] on the store page, given the [version] read. */
    suspend operator fun invoke(
        id: String,
        published: Boolean,
        version: Int,
    ): Item = items.setItemPublished(id, published, version)
}

/** ArchiveItem takes an item out of the catalog ("Arquivar"), or brings it back ("Restaurar"). */
class ArchiveItem(
    private val items: ItemRepository,
) {
    /** Archives the item [id]; throws `Failure.ItemInKit` while a kit that is not archived holds it. */
    suspend operator fun invoke(
        id: String,
        version: Int,
    ): Item = items.archiveItem(id, version)

    /** Brings the archived item [id] back, unpublished. */
    suspend fun restore(
        id: String,
        version: Int,
    ): Item = items.restoreItem(id, version)
}
