package br.com.encomendo.core.platform

import android.content.Context
import android.net.Uri
import androidx.activity.ComponentActivity
import androidx.activity.result.ActivityResultLauncher
import androidx.activity.result.PickVisualMediaRequest
import androidx.activity.result.contract.ActivityResultContracts
import androidx.core.content.FileProvider
import br.com.encomendo.core.domain.catalog.PickedPhoto
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.withContext
import java.io.File
import java.io.IOException
import java.util.UUID

/**
 * AndroidPhotoPicker is [PhotoPicker] over the system photo picker (no storage
 * permission) and the camera app. It needs an activity to launch from:
 * the activity hands itself over with [attach] in `onCreate`, because Android only lets
 * result launchers be registered before the activity starts. Without one it
 * returns null, as when the user backs out.
 *
 * Kept thin on purpose: it launches, waits for the answer and reads the
 * bytes. The camera writes to a private cache file shared through
 * `FileProvider` (see the manifest) and the file is deleted once read.
 *
 * @param ioDispatcher where the file is read; injected so a test can replace it.
 */
class AndroidPhotoPicker(
    private val context: Context,
    private val ioDispatcher: CoroutineDispatcher,
) : PhotoPicker {
    private var launchers: Launchers? = null
    private var waiting: CompletableDeferred<Uri?>? = null
    private var cameraFile: File? = null

    /** Registers the launchers on [activity]; call it in `onCreate`, once per activity instance. */
    fun attach(activity: ComponentActivity) {
        val gallery =
            activity.registerForActivityResult(ActivityResultContracts.PickVisualMedia()) { answer(it) }
        val camera =
            activity.registerForActivityResult(ActivityResultContracts.TakePicture()) { taken ->
                answer(if (taken) cameraFile?.let(::uriOf) else null)
            }
        launchers = Launchers(gallery, camera)
    }

    override suspend fun pickFromGallery(): PickedPhoto? {
        val launcher = launchers?.gallery ?: return null
        val uri = ask { launcher.launch(PickVisualMediaRequest(ActivityResultContracts.PickVisualMedia.ImageOnly)) }
        return uri?.let { read(it, context.contentResolver.getType(it) ?: JPEG) }
    }

    override suspend fun takePhoto(): PickedPhoto? {
        val launcher = launchers?.camera ?: return null
        val file = newCameraFile().also { cameraFile = it }
        val uri = ask { launcher.launch(uriOf(file)) }
        return try {
            uri?.let { read(it, JPEG) }
        } finally {
            file.delete()
            cameraFile = null
        }
    }

    private suspend fun ask(launch: () -> Unit): Uri? {
        val answer = CompletableDeferred<Uri?>()
        waiting = answer
        launch()
        return answer.await()
    }

    private fun answer(uri: Uri?) {
        waiting?.complete(uri)
        waiting = null
    }

    private suspend fun read(
        uri: Uri,
        contentType: String,
    ): PickedPhoto? =
        withContext(ioDispatcher) {
            val bytes = context.contentResolver.openInputStream(uri)?.use { it.readBytes() }
            bytes?.let { PickedPhoto(bytes = it, contentType = contentType) }
        }

    private fun newCameraFile(): File {
        val folder = File(context.cacheDir, "photos")
        if (!folder.isDirectory && !folder.mkdirs()) throw IOException("cannot create the photo folder")
        return File(folder, "camera-${UUID.randomUUID()}.jpg")
    }

    private fun uriOf(file: File): Uri = FileProvider.getUriForFile(context, "${context.packageName}.photos", file)

    private class Launchers(
        val gallery: ActivityResultLauncher<PickVisualMediaRequest>,
        val camera: ActivityResultLauncher<Uri>,
    )

    private companion object {
        const val JPEG = "image/jpeg"
    }
}
