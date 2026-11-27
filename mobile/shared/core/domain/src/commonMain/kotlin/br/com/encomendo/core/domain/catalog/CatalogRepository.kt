package br.com.encomendo.core.domain.catalog

import br.com.encomendo.core.domain.paging.Page

/**
 * Fetched is what a catalog read returns: the [value] and, when the call
 * could not reach the server, the moment the copy kept on the device was
 * saved. A screen shows its offline banner over a value whose [cachedAtEpochMillis]
 * is not null.
 */
data class Fetched<out T>(
    /** What was read. */
    val value: T,
    /** When the device's copy was saved, in epoch milliseconds; null when the server just answered. */
    val cachedAtEpochMillis: Long? = null,
) {
    /** True when the server did not answer and [value] is the device's last copy. */
    val fromCache: Boolean get() = cachedAtEpochMillis != null
}

/** ItemFilter narrows the Itens list: the same filters as the web admin. */
data class ItemFilter(
    /** Only items whose name contains this text, ignoring case; null for all. */
    val query: String? = null,
    /** Only items of this category; null for all. */
    val categoryId: String? = null,
    /** Also list archived items ("Mostrar arquivados"). */
    val includeArchived: Boolean = false,
)

/** KitFilter narrows the Kits list. */
data class KitFilter(
    /** Only kits whose name contains this text, ignoring case; null for all. */
    val query: String? = null,
    /** Also list archived kits ("Mostrar arquivados"). */
    val includeArchived: Boolean = false,
)

/**
 * CatalogRepository is the store's catalog: categories, fields, items, kits
 * and their photos, through the admin endpoints (`catalog` in
 * docs/api/openapi.yaml). Reading needs `catalog:read` (every role), writing
 * `catalog:write` (owner, admin); a call the role may not make throws
 * `Failure.Forbidden`.
 *
 * Costs and margins arrive as [OwnerField]s: [OwnerField.Hidden] for every
 * role but the owner. A write never carries a cost the user did not set
 * (ADR 0025). Reads fall back to the copy kept on the device when the call
 * cannot reach the server, and say so in [Fetched]; writes never do.
 */
interface CatalogRepository :
    CategoryRepository,
    AttributeRepository,
    ItemRepository,
    KitRepository,
    PhotoRepository

/** CategoryRepository reads and writes the store's categories. */
interface CategoryRepository {
    /** Returns every category in the store's order; a store has few, so there is no paging. */
    suspend fun categories(): Fetched<List<Category>>

    /** Adds a category at the end. Throws `Failure.AlreadyTaken` (field `name`) when the name is in use. */
    suspend fun createCategory(name: String): Category

    /** Renames the category [id]. Throws `Failure.AlreadyTaken` (field `name`) when the name is in use. */
    suspend fun renameCategory(
        id: String,
        name: String,
    ): Category

    /** Moves the category [id] to [position]; a position past the end moves it to the end. */
    suspend fun moveCategory(
        id: String,
        position: Int,
    ): Category

    /** Deletes the category [id]. Throws `Failure.CategoryInUse` while items, archived ones included, use it. */
    suspend fun deleteCategory(id: String)
}

/** AttributeRepository reads and writes the item fields ("Serve em" …) and their values. */
interface AttributeRepository {
    /** Returns every field in the store's order, with its values. */
    suspend fun attributes(): Fetched<List<Attribute>>

    /** Adds a field with its values. Throws `Failure.AlreadyTaken` (field `name`) when the name is in use. */
    suspend fun createAttribute(draft: AttributeDraft): Attribute

    /**
     * Replaces the field's name and values. A value left out leaves every item and kit that used it; ask first,
     * using [AttributeValue.usageCount].
     */
    suspend fun replaceAttribute(
        id: String,
        draft: AttributeDraft,
    ): Attribute

    /** Deletes the field; its values leave every item and kit. */
    suspend fun deleteAttribute(id: String)
}

/** ItemRepository reads and writes catalog items. */
interface ItemRepository {
    /** Returns up to [limit] items by name from [cursor] on (null for the first page). */
    suspend fun itemsPage(
        filter: ItemFilter,
        cursor: String?,
        limit: Int,
    ): Fetched<Page<ItemSummary>>

    /** Returns the item [id]. Throws `Failure.NotFound` when it is gone. */
    suspend fun item(id: String): Fetched<Item>

    /** Adds an item and returns it. Throws `Failure.InvalidInput` with the fields the API refused. */
    suspend fun createItem(draft: ItemDraft): Item

    /** Replaces the item's details, given the [version] read; throws `Failure.VersionConflict` if it moved on. */
    suspend fun replaceItem(
        id: String,
        version: Int,
        draft: ItemDraft,
    ): Item

    /** Shows or hides the item on the store page ("No portal"). */
    suspend fun setItemPublished(
        id: String,
        published: Boolean,
        version: Int,
    ): Item

    /** Archives the item. Throws `Failure.ItemInKit` naming the kits that hold it. */
    suspend fun archiveItem(
        id: String,
        version: Int,
    ): Item

    /** Brings an archived item back, unpublished. */
    suspend fun restoreItem(
        id: String,
        version: Int,
    ): Item
}

/** KitRepository reads and writes kits ("Kit pronto"). */
interface KitRepository {
    /** Returns up to [limit] kits by name from [cursor] on (null for the first page). */
    suspend fun kitsPage(
        filter: KitFilter,
        cursor: String?,
        limit: Int,
    ): Fetched<Page<KitSummary>>

    /** Returns the kit [id]. Throws `Failure.NotFound` when it is gone. */
    suspend fun kit(id: String): Fetched<Kit>

    /** Adds a kit and returns it. Throws `Failure.ItemArchived` when an item in it is archived. */
    suspend fun createKit(draft: KitDraft): Kit

    /** Replaces the kit, given the [version] read. Throws `Failure.VersionConflict` or `Failure.ItemArchived`. */
    suspend fun replaceKit(
        id: String,
        version: Int,
        draft: KitDraft,
    ): Kit

    /** Shows or hides the kit on the store page ("No portal"). */
    suspend fun setKitPublished(
        id: String,
        published: Boolean,
        version: Int,
    ): Kit

    /** Archives the kit. */
    suspend fun archiveKit(
        id: String,
        version: Int,
    ): Kit

    /** Brings an archived kit back, unpublished. Throws `Failure.ItemArchived` while an item of it is archived. */
    suspend fun restoreKit(
        id: String,
        version: Int,
    ): Kit
}

/** PhotoRepository uploads and arranges the photos of items and kits (up to 8 each; the first is the cover). */
interface PhotoRepository {
    /**
     * Uploads [photo] to the item or kit [owner] and returns it, last in the order. Throws `Failure.PhotoLimit`
     * with 8 photos already, and `Failure.PhotoInvalid` when the file is not a JPEG, PNG or WebP of at most 10 MB.
     */
    suspend fun uploadPhoto(
        owner: PhotoOwner,
        photo: PickedPhoto,
    ): Photo

    /** Puts the owner's photos in the order of [photoIds], every photo once; the first is the cover. */
    suspend fun orderPhotos(
        owner: PhotoOwner,
        photoIds: List<String>,
    ): List<Photo>

    /** Removes the photo [photoId] from the owner and from storage. */
    suspend fun deletePhoto(
        owner: PhotoOwner,
        photoId: String,
    )
}
