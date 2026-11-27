package br.com.encomendo.core.testing

import br.com.encomendo.core.domain.catalog.Attribute
import br.com.encomendo.core.domain.catalog.AttributeDraft
import br.com.encomendo.core.domain.catalog.AttributeRepository
import br.com.encomendo.core.domain.catalog.AttributeValue
import br.com.encomendo.core.domain.catalog.CatalogRepository
import br.com.encomendo.core.domain.catalog.Category
import br.com.encomendo.core.domain.catalog.CategoryRepository
import br.com.encomendo.core.domain.catalog.Fetched
import br.com.encomendo.core.domain.catalog.Item
import br.com.encomendo.core.domain.catalog.ItemRepository
import br.com.encomendo.core.domain.catalog.Kit
import br.com.encomendo.core.domain.catalog.KitRepository
import br.com.encomendo.core.domain.catalog.Photo
import br.com.encomendo.core.domain.catalog.PhotoOwner
import br.com.encomendo.core.domain.catalog.PhotoOwnerKind
import br.com.encomendo.core.domain.catalog.PhotoRepository
import br.com.encomendo.core.domain.catalog.PickedPhoto
import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.domain.supplier.Supplier
import kotlin.time.Instant

/**
 * FakeCatalogState is the catalog a [FakeCatalogRepository] serves, as the
 * server would hold it: a test seeds [categories], [attributes], [items] and
 * [kits], reads them back after a write, and sets [failure] to make every
 * call fail with a `Failure`, or [offlineSince] to make reads answer as the
 * device's cached copy.
 */
class FakeCatalogState {
    /** The store's categories, in order. */
    var categories: List<Category> = emptyList()

    /** The store's item fields. */
    var attributes: List<Attribute> = emptyList()

    /** Every item, archived ones included. */
    var items: List<Item> = emptyList()

    /** Every kit, archived ones included. */
    var kits: List<Kit> = emptyList()

    /** The store's suppliers, which an item's supplier links name; [FakeSupplierRepository] serves the same list. */
    var suppliers: List<Supplier> = emptyList()

    /** Whether the faked role sees costs (the owner): items then carry them; any other role is refused one. */
    var seesCosts: Boolean = false

    /** When set, every call throws it, as an API refusal or a lost signal would. */
    var failure: Failure? = null

    /** When set, reads answer as the device's copy saved at this time (epoch milliseconds). */
    var offlineSince: Long? = null

    private var nextId = 1

    /** Throws [failure] when set; every fake call starts here. */
    fun check() {
        failure?.let { throw it }
    }

    /** Wraps a read, marking it as cached when [offlineSince] is set. */
    fun <T> read(value: T): Fetched<T> {
        check()
        return Fetched(value, offlineSince)
    }

    /** Returns an id no seeded row has: `fake-1`, `fake-2` … */
    fun newId(): String = "fake-${nextId++}"
}

/**
 * FakeCatalogRepository answers as the API would, in memory: categories, item fields, items, kits and photos
 * behave as their facets document, over one [state] a test seeds and inspects.
 */
class FakeCatalogRepository(
    /** The catalog this fake serves and changes. */
    val state: FakeCatalogState = FakeCatalogState(),
) : CatalogRepository,
    CategoryRepository by FakeCategoryRepository(state),
    AttributeRepository by FakeAttributeRepository(state),
    ItemRepository by FakeItemRepository(state),
    KitRepository by FakeKitRepository(state),
    PhotoRepository by FakePhotoRepository(state)

/** The fake's categories: unique names, and none deleted while an item uses it. */
class FakeCategoryRepository(
    private val state: FakeCatalogState,
) : CategoryRepository {
    override suspend fun categories(): Fetched<List<Category>> = state.read(state.categories)

    override suspend fun createCategory(name: String): Category {
        state.check()
        if (state.categories.any { it.name.equals(name, ignoreCase = true) }) throw Failure.AlreadyTaken("name")
        val created = Category(state.newId(), name, position = state.categories.size)
        state.categories = state.categories + created
        return created
    }

    override suspend fun renameCategory(
        id: String,
        name: String,
    ): Category = change(id) { it.copy(name = name) }

    override suspend fun moveCategory(
        id: String,
        position: Int,
    ): Category {
        val moved = change(id) { it.copy(position = position.coerceAtMost(state.categories.lastIndex)) }
        // Numbered again from 0, as the server does after a move.
        val ordered = state.categories.sortedBy { it.position }
        state.categories = ordered.mapIndexed { index, category -> category.copy(position = index) }
        return state.categories.first { it.id == moved.id }
    }

    override suspend fun deleteCategory(id: String) {
        state.check()
        if (state.items.any { it.categoryId == id }) throw Failure.CategoryInUse()
        state.categories = state.categories.filterNot { it.id == id }
    }

    private fun change(
        id: String,
        edit: (Category) -> Category,
    ): Category {
        state.check()
        val changed = edit(state.categories.firstOrNull { it.id == id } ?: throw Failure.NotFound())
        state.categories = state.categories.map { if (it.id == id) changed else it }
        return changed
    }
}

/** The fake's item fields. A value with an id keeps it; one without gets a new id. */
class FakeAttributeRepository(
    private val state: FakeCatalogState,
) : AttributeRepository {
    override suspend fun attributes(): Fetched<List<Attribute>> = state.read(state.attributes)

    override suspend fun createAttribute(draft: AttributeDraft): Attribute {
        state.check()
        if (state.attributes.any { it.name.equals(draft.name, ignoreCase = true) }) throw Failure.AlreadyTaken("name")
        val created = build(state.newId(), state.attributes.size, draft)
        state.attributes = state.attributes + created
        return created
    }

    override suspend fun replaceAttribute(
        id: String,
        draft: AttributeDraft,
    ): Attribute {
        state.check()
        val old = state.attributes.firstOrNull { it.id == id } ?: throw Failure.NotFound()
        val replaced = build(id, old.position, draft, old.values)
        state.attributes = state.attributes.map { if (it.id == id) replaced else it }
        return replaced
    }

    override suspend fun deleteAttribute(id: String) {
        state.check()
        state.attributes = state.attributes.filterNot { it.id == id }
    }

    private fun build(
        id: String,
        position: Int,
        draft: AttributeDraft,
        old: List<AttributeValue> = emptyList(),
    ): Attribute =
        Attribute(
            id = id,
            name = draft.name,
            position = position,
            values =
                draft.values.map { value ->
                    val kept = old.firstOrNull { it.id == value.id }
                    AttributeValue(kept?.id ?: state.newId(), value.label, kept?.usageCount ?: 0)
                },
        )
}

/** The fake's photos: up to 8 per item or kit, JPEG, PNG or WebP, the first one the cover. */
class FakePhotoRepository(
    private val state: FakeCatalogState,
) : PhotoRepository {
    override suspend fun uploadPhoto(
        owner: PhotoOwner,
        photo: PickedPhoto,
    ): Photo {
        state.check()
        if (photo.contentType !in ACCEPTED_TYPES) throw Failure.PhotoInvalid()
        val photos = photosOf(owner)
        if (photos.size >= MAX_PHOTOS) throw Failure.PhotoLimit()
        val added = Photo(state.newId(), "https://files.test/${photos.size}", Instant.fromEpochSeconds(0), photos.size)
        save(owner, photos + added)
        return added
    }

    override suspend fun orderPhotos(
        owner: PhotoOwner,
        photoIds: List<String>,
    ): List<Photo> {
        state.check()
        val photos = photosOf(owner)
        val ordered = photoIds.map { id -> photos.firstOrNull { it.id == id } ?: throw Failure.NotFound() }
        return ordered.mapIndexed { index, photo -> photo.copy(position = index) }.also { save(owner, it) }
    }

    override suspend fun deletePhoto(
        owner: PhotoOwner,
        photoId: String,
    ) {
        state.check()
        val photos = photosOf(owner)
        if (photos.none { it.id == photoId }) throw Failure.NotFound()
        save(owner, photos.filterNot { it.id == photoId }.mapIndexed { index, photo -> photo.copy(position = index) })
    }

    private fun photosOf(owner: PhotoOwner): List<Photo> =
        when (owner.kind) {
            PhotoOwnerKind.Item -> state.items.firstOrNull { it.id == owner.id }?.photos
            PhotoOwnerKind.Kit -> state.kits.firstOrNull { it.id == owner.id }?.photos
        } ?: throw Failure.NotFound()

    private fun save(
        owner: PhotoOwner,
        photos: List<Photo>,
    ) {
        when (owner.kind) {
            PhotoOwnerKind.Item ->
                state.items = state.items.map { if (it.id == owner.id) it.copy(photos = photos) else it }
            PhotoOwnerKind.Kit ->
                state.kits = state.kits.map { if (it.id == owner.id) it.copy(photos = photos) else it }
        }
    }

    private companion object {
        const val MAX_PHOTOS = 8
        val ACCEPTED_TYPES = setOf("image/jpeg", "image/png", "image/webp")
    }
}
