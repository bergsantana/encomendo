package br.com.encomendo.feature.catalogo

import br.com.encomendo.core.domain.catalog.ChangePhotos
import br.com.encomendo.core.domain.catalog.Photo
import br.com.encomendo.core.domain.catalog.PhotoOwner
import br.com.encomendo.core.domain.catalog.PhotoOwnerKind
import br.com.encomendo.core.domain.catalog.PickedPhoto
import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.platform.PhotoPicker
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.launch

/** PhotoSource says where "Adicionar fotos" gets a photo. */
enum class PhotoSource {
    /** The phone's gallery. */
    Gallery,

    /** The camera. */
    Camera,
}

/**
 * PhotosState is the "Fotos" section of an item or kit: its photos (the first is the cover), whether they may
 * change, and what the last change said. A photo change goes through at once, not with "Salvar".
 */
data class PhotosState(
    /** Whether the photos belong to an item or to a kit, which decides the sentences that name which. */
    val ownerKind: PhotoOwnerKind = PhotoOwnerKind.Item,
    /** The id of the item or kit the photos belong to; null while it is not saved, so there is nothing to upload to. */
    val ownerId: String? = null,
    /** The photos, the first being the cover. */
    val photos: List<Photo> = emptyList(),
    /** The role may change them (`catalog:write`) and the owner was read. */
    val enabled: Boolean = false,
    /** A photo is being uploaded, ordered or removed; every photo button waits. */
    val busy: Boolean = false,
    /** The upload itself is under way: "Enviando a foto…". */
    val uploading: Boolean = false,
    /** Why the last change failed, under "Fotos". */
    val message: CatalogMessage? = null,
) {
    /** A photo change may start: a saved owner the role may change, with no other change under way. */
    val canChange: Boolean get() = enabled && ownerId != null && !busy
}

/** StateSlot is the part of a screen's state a helper changes, so the helper never owns a flow of its own. */
class StateSlot<T>(
    private val read: () -> T,
    private val write: ((T) -> T) -> Unit,
) {
    /** The current value. */
    val value: T get() = read()

    /** Replaces the value with [change] of it. */
    fun update(change: (T) -> T) = write(change)
}

/**
 * PhotoController adds, orders and removes the photos of one item or kit, whose [kind] it is given, so the item form
 * and the kit editor (step 3.16) share it. It changes only [slot], in [scope], which is the ViewModel's. A photo the
 * picker refuses for size, or the API refuses, says so in [PhotosState.message].
 */
class PhotoController(
    private val scope: CoroutineScope,
    private val kind: PhotoOwnerKind,
    private val useCases: ChangePhotos,
    private val picker: PhotoPicker,
    private val slot: StateSlot<PhotosState>,
) {
    /** Starts showing [photos] of the owner [ownerId] (null while it is not saved); they change only if [enabled]. */
    fun open(
        ownerId: String?,
        photos: List<Photo>,
        enabled: Boolean,
    ) = slot.update { PhotosState(ownerKind = kind, ownerId = ownerId, photos = photos, enabled = enabled) }

    /** "Adicionar fotos" with [source]. Nothing happens when the user backs out. */
    fun add(source: PhotoSource) {
        val state = slot.value
        val owner = ownerOf(state) ?: return
        if (state.photos.size >= MAX_PHOTOS) {
            slot.update { it.copy(message = failureOf(Failure.PhotoLimit())) }
            return
        }
        changing {
            val picked = pick(source) ?: return@changing
            slot.update { it.copy(uploading = true) }
            val photo = useCases.upload(owner, picked)
            slot.update { it.copy(photos = (it.photos + photo).sortedBy(Photo::position)) }
        }
    }

    /** "Usar como capa": [photo] goes first. */
    fun makeCover(photo: Photo) {
        val state = slot.value
        val owner = ownerOf(state)
        if (owner == null || state.photos.firstOrNull()?.id == photo.id) return
        changing {
            val order = listOf(photo.id) + state.photos.map(Photo::id).filter { it != photo.id }
            val ordered = useCases.order(owner, order)
            slot.update { it.copy(photos = ordered) }
        }
    }

    /** "Remover foto". */
    fun remove(photo: Photo) {
        val owner = ownerOf(slot.value) ?: return
        changing {
            useCases.remove(owner, photo.id)
            slot.update { it.copy(photos = it.photos.filter { kept -> kept.id != photo.id }) }
        }
    }

    /** The message for [failure], worded for an item or a kit as the owner is. */
    private fun failureOf(failure: Failure): CatalogMessage =
        failure.toCatalogMessage(if (kind == PhotoOwnerKind.Kit) CatalogSubject.Kit else CatalogSubject.Item)

    private fun ownerOf(state: PhotosState): PhotoOwner? =
        state.ownerId?.takeIf { state.canChange }?.let { PhotoOwner(kind, it) }

    private fun changing(change: suspend () -> Unit) {
        slot.update { it.copy(busy = true, message = null) }
        scope.launch {
            try {
                change()
            } catch (failure: Failure) {
                slot.update { it.copy(message = failureOf(failure)) }
            } finally {
                slot.update { it.copy(busy = false, uploading = false) }
            }
        }
    }

    /** Asks the picker; a file over 10 MB is refused by the picker itself, and says so under "Fotos". */
    private suspend fun pick(source: PhotoSource): PickedPhoto? =
        try {
            when (source) {
                PhotoSource.Gallery -> picker.pickFromGallery()
                PhotoSource.Camera -> picker.takePhoto()
            }
        } catch (_: Failure.PhotoInvalid) {
            slot.update { it.copy(message = CatalogMessage.PhotoTooBig) }
            null
        }

    private companion object {
        const val MAX_PHOTOS = 8
    }
}
