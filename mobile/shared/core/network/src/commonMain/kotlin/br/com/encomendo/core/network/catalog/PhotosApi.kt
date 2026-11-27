package br.com.encomendo.core.network.catalog

import br.com.encomendo.core.network.decoded
import io.ktor.client.HttpClient
import io.ktor.client.request.delete
import io.ktor.client.request.post
import io.ktor.client.request.put
import io.ktor.http.encodeURLPathPart
import kotlinx.serialization.Serializable

/** PhotoOwnerType says which path and `ownerType` a photo call uses. */
enum class PhotoOwnerType(
    internal val segment: String,
    internal val wire: String,
) {
    /** `/items/{id}/photos`, `ownerType: item`. */
    Item("items", "item"),

    /** `/kits/{id}/photos`, `ownerType: kit`. */
    Kit("kits", "kit"),
}

/**
 * PhotosApi calls the photo endpoints of items and kits (docs/api/openapi.yaml,
 * `catalog`): ask where to upload, confirm, order and remove. The upload
 * itself goes to the storage host through [PhotoStorageApi], never here.
 * Calls need `catalog:write`.
 */
class PhotosApi(
    private val client: HttpClient,
) {
    /**
     * Asks where to upload one photo of the item or kit [ownerId]. The server picks the key; the answer says
     * how to send the bytes.
     */
    suspend fun requestUpload(
        owner: PhotoOwnerType,
        ownerId: String,
        contentType: String,
    ): PhotoUploadDto =
        client
            .post("$CATALOG/photo-uploads") {
                jsonBody(PhotoUploadRequestDto(ownerType = owner.wire, ownerId = ownerId, contentType = contentType))
            }.decoded()

    /**
     * Records the photo uploaded to [key]. `201` for a new photo and `200` for a key already recorded are both
     * success.
     */
    suspend fun confirm(
        owner: PhotoOwnerType,
        ownerId: String,
        key: String,
    ): PhotoDto = client.post(photosPath(owner, ownerId)) { jsonBody(ConfirmPhotoDto(key)) }.decoded()

    /** Puts the owner's photos in the order of [photoIds]; the first is the cover. */
    suspend fun order(
        owner: PhotoOwnerType,
        ownerId: String,
        photoIds: List<String>,
    ): PhotoListDto =
        client
            .put("${photosPath(owner, ownerId)}/order") { jsonBody(OrderPhotosDto(photoIds)) }
            .decoded()

    /** Removes the photo [photoId]; the answer is 204 with no body. */
    suspend fun delete(
        owner: PhotoOwnerType,
        ownerId: String,
        photoId: String,
    ) {
        client.delete("${photosPath(owner, ownerId)}/${photoId.encodeURLPathPart()}")
    }

    private fun photosPath(
        owner: PhotoOwnerType,
        ownerId: String,
    ): String = "$CATALOG/${owner.segment}/${ownerId.encodeURLPathPart()}/photos"
}

@Serializable
private class PhotoUploadRequestDto(
    val ownerType: String,
    val ownerId: String,
    val contentType: String,
)

@Serializable
private class ConfirmPhotoDto(
    val key: String,
)

@Serializable
private class OrderPhotosDto(
    val photoIds: List<String>,
)
