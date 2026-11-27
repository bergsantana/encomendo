package br.com.encomendo.core.domain.catalog

import kotlin.time.Instant

/** Photo is one picture of an item or kit, with a short-lived signed URL to read it. */
data class Photo(
    /** The photo's id (a UUID). */
    val id: String,
    /** A signed URL to read the photo; read the item again after [expiresAt]. */
    val url: String,
    /** When [url] stops working. */
    val expiresAt: Instant,
    /** Its place among the owner's photos; 0 is the cover. */
    val position: Int,
)

/** PhotoOwnerKind says whether a photo belongs to an item or to a kit. */
enum class PhotoOwnerKind {
    /** A catalog item. */
    Item,

    /** A kit. */
    Kit,
}

/** PhotoOwner names the item or kit a photo belongs to. */
data class PhotoOwner(
    /** Whether [id] is an item's or a kit's. */
    val kind: PhotoOwnerKind,
    /** The item's or kit's id (a UUID). */
    val id: String,
)

/**
 * PickedPhoto is the file the user chose or took: its [bytes] and the
 * [contentType] the platform reported. The API accepts JPEG, PNG and WebP
 * of at most 10 MB (ADR 0024).
 */
class PickedPhoto(
    /** The file's content. */
    val bytes: ByteArray,
    /** The MIME type, such as `image/jpeg`. */
    val contentType: String,
) {
    /** Prints only the size, so a log line never carries the picture. */
    override fun toString(): String = "PickedPhoto(${bytes.size} bytes, $contentType)"
}
