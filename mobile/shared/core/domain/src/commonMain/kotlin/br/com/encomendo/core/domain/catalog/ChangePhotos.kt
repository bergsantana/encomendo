package br.com.encomendo.core.domain.catalog

/**
 * ChangePhotos uploads, orders and removes the photos of an item or a kit, which share the same calls. Thin on
 * purpose: the repository owns the limit of 8 and the checks on the file.
 */
class ChangePhotos(
    private val photos: PhotoRepository,
) {
    /** Uploads [photo] to [owner], last in the order; throws `Failure.PhotoLimit` or `Failure.PhotoInvalid`. */
    suspend fun upload(
        owner: PhotoOwner,
        photo: PickedPhoto,
    ): Photo = photos.uploadPhoto(owner, photo)

    /** Puts the owner's photos in the order of [photoIds]; the first is the cover. */
    suspend fun order(
        owner: PhotoOwner,
        photoIds: List<String>,
    ): List<Photo> = photos.orderPhotos(owner, photoIds)

    /** Removes the photo [photoId] from [owner]. */
    suspend fun remove(
        owner: PhotoOwner,
        photoId: String,
    ) = photos.deletePhoto(owner, photoId)
}
