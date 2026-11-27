package br.com.encomendo.core.domain.catalog

import br.com.encomendo.core.domain.paging.Page

// The kit use cases are thin on purpose, like the item ones: the repository owns the rules (versions, archived
// items, totals), and the session-wide failures are handled below it, in core:data's StoreCalls.

/** ListKits reads a page of the Kits list. */
class ListKits(
    private val kits: KitRepository,
) {
    /** Returns up to [limit] kits matching [filter] from [cursor] on; null [cursor] reads the first page. */
    suspend operator fun invoke(
        filter: KitFilter,
        cursor: String?,
        limit: Int,
    ): Fetched<Page<KitSummary>> = kits.kitsPage(filter, cursor, limit)
}

/** GetKit reads one kit for the kit editor. */
class GetKit(
    private val kits: KitRepository,
) {
    /** Returns the kit [id] with its totals and days; throws `Failure.NotFound` when it is gone. */
    suspend operator fun invoke(id: String): Fetched<Kit> = kits.kit(id)
}

/** SaveKit creates a kit, or replaces one against the version the user read ("Salvar"). */
class SaveKit(
    private val kits: KitRepository,
) {
    /** Creates the kit [draft] and returns it; throws `Failure.ItemArchived` when an item in it is archived. */
    suspend fun create(draft: KitDraft): Kit = kits.createKit(draft)

    /** Replaces the kit [id] given the [version] read; throws `Failure.VersionConflict` if it moved on. */
    suspend fun replace(
        id: String,
        version: Int,
        draft: KitDraft,
    ): Kit = kits.replaceKit(id, version, draft)
}

/** SetKitPublished flips "No portal" for a kit. */
class SetKitPublished(
    private val kits: KitRepository,
) {
    /** Shows or hides the kit [id] on the store page, given the [version] read. */
    suspend operator fun invoke(
        id: String,
        published: Boolean,
        version: Int,
    ): Kit = kits.setKitPublished(id, published, version)
}

/** ArchiveKit takes a kit out of the catalog ("Arquivar"), or brings it back ("Restaurar"). */
class ArchiveKit(
    private val kits: KitRepository,
) {
    /** Archives the kit [id]; it leaves the store page and stops holding its items. */
    suspend operator fun invoke(
        id: String,
        version: Int,
    ): Kit = kits.archiveKit(id, version)

    /** Brings the archived kit [id] back, unpublished; throws `Failure.ItemArchived` while an item is archived. */
    suspend fun restore(
        id: String,
        version: Int,
    ): Kit = kits.restoreKit(id, version)
}
