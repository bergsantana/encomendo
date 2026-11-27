package br.com.encomendo.core.data.catalog

import br.com.encomendo.core.data.failure.StoreCalls
import br.com.encomendo.core.database.CatalogCacheDao
import br.com.encomendo.core.domain.catalog.Attribute
import br.com.encomendo.core.domain.catalog.AttributeDraft
import br.com.encomendo.core.domain.catalog.AttributeRepository
import br.com.encomendo.core.domain.catalog.CatalogRepository
import br.com.encomendo.core.domain.catalog.Category
import br.com.encomendo.core.domain.catalog.CategoryRepository
import br.com.encomendo.core.domain.catalog.Fetched
import br.com.encomendo.core.domain.catalog.Item
import br.com.encomendo.core.domain.catalog.ItemDraft
import br.com.encomendo.core.domain.catalog.ItemFilter
import br.com.encomendo.core.domain.catalog.ItemRepository
import br.com.encomendo.core.domain.catalog.ItemSummary
import br.com.encomendo.core.domain.catalog.Kit
import br.com.encomendo.core.domain.catalog.KitDraft
import br.com.encomendo.core.domain.catalog.KitFilter
import br.com.encomendo.core.domain.catalog.KitRepository
import br.com.encomendo.core.domain.catalog.KitSummary
import br.com.encomendo.core.domain.catalog.Photo
import br.com.encomendo.core.domain.catalog.PhotoOwner
import br.com.encomendo.core.domain.catalog.PhotoOwnerKind
import br.com.encomendo.core.domain.catalog.PhotoRepository
import br.com.encomendo.core.domain.catalog.PickedPhoto
import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.domain.paging.Page
import br.com.encomendo.core.domain.session.SessionRepository
import br.com.encomendo.core.network.catalog.AttributeListDto
import br.com.encomendo.core.network.catalog.CatalogApis
import br.com.encomendo.core.network.catalog.CategoriesApi
import br.com.encomendo.core.network.catalog.CategoryListDto
import br.com.encomendo.core.network.catalog.ItemDto
import br.com.encomendo.core.network.catalog.ItemPageDto
import br.com.encomendo.core.network.catalog.ItemsApi
import br.com.encomendo.core.network.catalog.KitDto
import br.com.encomendo.core.network.catalog.KitPageDto
import br.com.encomendo.core.network.catalog.KitsApi
import br.com.encomendo.core.network.catalog.PhotoOwnerType
import br.com.encomendo.core.network.catalog.PhotoStorageApi
import br.com.encomendo.core.network.catalog.PhotosApi
import kotlin.time.Clock

/**
 * ApiCatalogRepository is the store's catalog through [api]: one object over
 * the facets below, so a ViewModel can take the whole [CatalogRepository] or
 * just the part it needs. Every call goes through [StoreCalls.inStore]; reads
 * fall back to the offline cache kept in [cache], and photo bytes go to the
 * storage host through [storage], never through [api] (ADR 0024).
 */
class ApiCatalogRepository(
    apis: CatalogApis,
    storage: PhotoStorageApi,
    calls: StoreCalls,
    cache: CatalogCacheDao,
    sessions: SessionRepository,
    clock: Clock = Clock.System,
) : CatalogRepository,
    CategoryRepository by ApiCategoryRepository(apis.categories, reads(calls, cache, sessions, clock)),
    AttributeRepository by ApiAttributeRepository(apis.categories, reads(calls, cache, sessions, clock)),
    ItemRepository by ApiItemRepository(apis.items, reads(calls, cache, sessions, clock)),
    KitRepository by ApiKitRepository(apis.kits, reads(calls, cache, sessions, clock)),
    PhotoRepository by ApiPhotoRepository(apis.photos, storage, calls)

private fun reads(
    calls: StoreCalls,
    cache: CatalogCacheDao,
    sessions: SessionRepository,
    clock: Clock,
): CatalogReads = CatalogReads(calls, CatalogCache(cache, sessions, clock))

internal class ApiCategoryRepository(
    private val api: CategoriesApi,
    private val reads: CatalogReads,
) : CategoryRepository {
    override suspend fun categories(): Fetched<List<Category>> =
        reads.list(
            kind = "categories",
            query = "",
            cursor = null,
            serializer = CategoryListDto.serializer(),
            fetch = api::categories,
            map = { list -> list.items.map { it.toCategory() } },
        )

    override suspend fun createCategory(name: String): Category = reads.write { api.createCategory(name).toCategory() }

    override suspend fun renameCategory(
        id: String,
        name: String,
    ): Category = reads.write { api.updateCategory(id, name = name, position = null).toCategory() }

    override suspend fun moveCategory(
        id: String,
        position: Int,
    ): Category = reads.write { api.updateCategory(id, name = null, position = position).toCategory() }

    override suspend fun deleteCategory(id: String) = reads.write { api.deleteCategory(id) }
}

internal class ApiAttributeRepository(
    private val api: CategoriesApi,
    private val reads: CatalogReads,
) : AttributeRepository {
    override suspend fun attributes(): Fetched<List<Attribute>> =
        reads.list(
            kind = "attributes",
            query = "",
            cursor = null,
            serializer = AttributeListDto.serializer(),
            fetch = api::attributes,
            map = { list -> list.items.map { it.toAttribute() } },
        )

    override suspend fun createAttribute(draft: AttributeDraft): Attribute =
        reads.write { api.createAttribute(draft.toWrite()).toAttribute() }

    override suspend fun replaceAttribute(
        id: String,
        draft: AttributeDraft,
    ): Attribute = reads.write { api.replaceAttribute(id, draft.toWrite()).toAttribute() }

    override suspend fun deleteAttribute(id: String) = reads.write { api.deleteAttribute(id) }
}

internal class ApiItemRepository(
    private val api: ItemsApi,
    private val reads: CatalogReads,
) : ItemRepository {
    override suspend fun itemsPage(
        filter: ItemFilter,
        cursor: String?,
        limit: Int,
    ): Fetched<Page<ItemSummary>> =
        reads.list(
            kind = "items",
            query = filter.cacheKey(),
            cursor = cursor,
            serializer = ItemPageDto.serializer(),
            fetch = { api.list(filter.query, filter.categoryId, filter.includeArchived, cursor, limit) },
            map = { page -> Page(page.items.map { it.toItemSummary() }, page.nextCursor) },
        )

    override suspend fun item(id: String): Fetched<Item> =
        reads.detail(ITEM, id, ItemDto.serializer(), { api.get(id) }, ItemDto::toItem)

    override suspend fun createItem(draft: ItemDraft): Item = saved { api.create(draft.toWrite(version = null)) }

    override suspend fun replaceItem(
        id: String,
        version: Int,
        draft: ItemDraft,
    ): Item = saved { api.replace(id, draft.toWrite(version)) }

    override suspend fun setItemPublished(
        id: String,
        published: Boolean,
        version: Int,
    ): Item = saved { api.setPublished(id, published, version) }

    override suspend fun archiveItem(
        id: String,
        version: Int,
    ): Item = saved { api.archive(id, version) }

    override suspend fun restoreItem(
        id: String,
        version: Int,
    ): Item = saved { api.restore(id, version) }

    private suspend fun saved(call: suspend () -> ItemDto): Item =
        reads.write(ITEM, ItemDto.serializer(), ItemDto::id, call, ItemDto::toItem)

    private companion object {
        const val ITEM = "item"
    }
}

internal class ApiKitRepository(
    private val api: KitsApi,
    private val reads: CatalogReads,
) : KitRepository {
    override suspend fun kitsPage(
        filter: KitFilter,
        cursor: String?,
        limit: Int,
    ): Fetched<Page<KitSummary>> =
        reads.list(
            kind = "kits",
            query = filter.cacheKey(),
            cursor = cursor,
            serializer = KitPageDto.serializer(),
            fetch = { api.list(filter.query, filter.includeArchived, cursor, limit) },
            map = { page -> Page(page.items.map { it.toKitSummary() }, page.nextCursor) },
        )

    override suspend fun kit(id: String): Fetched<Kit> =
        reads.detail(KIT, id, KitDto.serializer(), { api.get(id) }, KitDto::toKit)

    override suspend fun createKit(draft: KitDraft): Kit = saved { api.create(draft.toWrite(version = null)) }

    override suspend fun replaceKit(
        id: String,
        version: Int,
        draft: KitDraft,
    ): Kit = saved { api.replace(id, draft.toWrite(version)) }

    override suspend fun setKitPublished(
        id: String,
        published: Boolean,
        version: Int,
    ): Kit = saved { api.setPublished(id, published, version) }

    override suspend fun archiveKit(
        id: String,
        version: Int,
    ): Kit = saved { api.archive(id, version) }

    override suspend fun restoreKit(
        id: String,
        version: Int,
    ): Kit = saved { api.restore(id, version) }

    private suspend fun saved(call: suspend () -> KitDto): Kit =
        reads.write(KIT, KitDto.serializer(), KitDto::id, call, KitDto::toKit)

    private companion object {
        const val KIT = "kit"
    }
}

/**
 * Photos in three steps (ADR 0024): ask the API where to upload, send the bytes to that signed URL with the
 * headers it returned, then confirm. The bytes go through [storage], a client with no bearer token.
 */
internal class ApiPhotoRepository(
    private val api: PhotosApi,
    private val storage: PhotoStorageApi,
    private val calls: StoreCalls,
) : PhotoRepository {
    override suspend fun uploadPhoto(
        owner: PhotoOwner,
        photo: PickedPhoto,
    ): Photo {
        // The API takes only these types; refusing here saves an upload the server would delete again.
        if (photo.contentType !in ACCEPTED_TYPES) throw Failure.PhotoInvalid()
        val type = owner.kind.toApi()
        val upload = calls.inStore { api.requestUpload(type, owner.id, photo.contentType) }
        calls.inStore { storage.put(upload.uploadUrl, upload.headers, photo.bytes) }
        return calls.inStore { api.confirm(type, owner.id, upload.key).toPhoto() }
    }

    override suspend fun orderPhotos(
        owner: PhotoOwner,
        photoIds: List<String>,
    ): List<Photo> = calls.inStore { api.order(owner.kind.toApi(), owner.id, photoIds).items.map { it.toPhoto() } }

    override suspend fun deletePhoto(
        owner: PhotoOwner,
        photoId: String,
    ) = calls.inStore { api.delete(owner.kind.toApi(), owner.id, photoId) }

    private fun PhotoOwnerKind.toApi(): PhotoOwnerType =
        when (this) {
            PhotoOwnerKind.Item -> PhotoOwnerType.Item
            PhotoOwnerKind.Kit -> PhotoOwnerType.Kit
        }

    private companion object {
        val ACCEPTED_TYPES = setOf("image/jpeg", "image/png", "image/webp")
    }
}

// The filters of a list, as the one string its cached pages are filed under.
private fun ItemFilter.cacheKey(): String = "q=${query.orEmpty()}&c=${categoryId.orEmpty()}&a=$includeArchived"

private fun KitFilter.cacheKey(): String = "q=${query.orEmpty()}&a=$includeArchived"
