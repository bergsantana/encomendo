package br.com.encomendo.core.platform

import br.com.encomendo.core.domain.catalog.PickedPhoto

/**
 * PhotoPicker gets a photo from the user for an item or kit (milestone 3;
 * mobile/CLAUDE.md, expect / actual). The platform shows its own picker or
 * camera; this interface only returns what the user chose.
 *
 * Both calls return null when the user backs out, and when the app has no
 * screen in front to ask on (it went to the background): nothing was chosen,
 * and the form simply stays as it was. The API decides the type (JPEG, PNG or
 * WebP) from the file's content; the picker only refuses a file over the API's
 * 10 MB limit, before reading it into memory, by throwing `Failure.PhotoInvalid`.
 */
interface PhotoPicker {
    /** Opens the system gallery picker and returns the photo chosen, or null. */
    suspend fun pickFromGallery(): PickedPhoto?

    /** Opens the camera and returns the photo taken, or null. */
    suspend fun takePhoto(): PickedPhoto?
}
