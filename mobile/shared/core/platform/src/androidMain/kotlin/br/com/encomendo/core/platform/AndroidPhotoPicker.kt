package br.com.encomendo.core.platform

import android.content.Context
import android.net.Uri
import android.provider.OpenableColumns
import androidx.activity.ComponentActivity
import androidx.activity.result.ActivityResultLauncher
import androidx.activity.result.PickVisualMediaRequest
import androidx.activity.result.contract.ActivityResultContracts
import androidx.core.content.FileProvider
import androidx.lifecycle.DefaultLifecycleObserver
import androidx.lifecycle.LifecycleOwner
import br.com.encomendo.core.domain.catalog.PickedPhoto
import br.com.encomendo.core.domain.failure.Failure
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.withContext
import java.io.ByteArrayOutputStream
import java.io.File
import java.io.IOException
import java.io.InputStream
import java.util.UUID

/**
 * AndroidPhotoPicker is [PhotoPicker] over the system photo picker (no storage
 * permission) and the camera app. It needs an activity to launch from:
 * the activity hands itself over with [attach] in `onCreate`, because Android only lets
 * result launchers be registered before the activity starts. Without one it
 * returns null, as when the user backs out.
 *
 * It is a process-wide singleton, so it lets go of an activity when that
 * activity is destroyed, instead of pinning it. A file over the API's 10 MB
 * limit is refused with `Failure.PhotoInvalid` before it is read into memory.
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
        val mine = Launchers(gallery, camera)
        launchers = mine
        activity.lifecycle.addObserver(
            object : DefaultLifecycleObserver {
                override fun onDestroy(owner: LifecycleOwner) {
                    release(mine, activity.isChangingConfigurations)
                }
            },
        )
    }

    // Lets go of a destroyed activity's launchers, unless a newer activity attached its own already. A pick still
    // open while the activity is recreated keeps waiting: its answer arrives at the new activity.
    private fun release(
        released: Launchers,
        recreating: Boolean,
    ) {
        if (launchers === released) launchers = null
        if (!recreating) answer(null)
    }

    override suspend fun pickFromGallery(): PickedPhoto? {
        val launcher = launchers?.gallery ?: return null
        val uri = ask { launcher.launch(PickVisualMediaRequest(ActivityResultContracts.PickVisualMedia.ImageOnly)) }
        return uri?.let { read(it, context.contentResolver.getType(it) ?: JPEG) }
    }

    override suspend fun takePhoto(): PickedPhoto? {
        val launcher = launchers?.camera ?: return null
        val file = newCameraFile().also { cameraFile = it }
        return try {
            ask { launcher.launch(uriOf(file)) }?.let { read(it, JPEG) }
        } finally {
            file.delete()
            // A second call may have taken over: leave its file alone.
            if (cameraFile === file) cameraFile = null
        }
    }

    // One question at a time: an earlier one that is still open is answered "nothing chosen", so it can never
    // receive the later one's result, and the deferred is forgotten when the asker is cancelled.
    private suspend fun ask(launch: () -> Unit): Uri? {
        val answer = CompletableDeferred<Uri?>()
        waiting?.complete(null)
        waiting = answer
        try {
            launch()
            return answer.await()
        } finally {
            if (waiting === answer) waiting = null
        }
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
            if ((sizeOf(uri) ?: 0L) > MAX_BYTES) throw Failure.PhotoInvalid()
            val bytes = context.contentResolver.openInputStream(uri)?.use(::readAtMost)
            bytes?.let { PickedPhoto(bytes = it, contentType = contentType) }
        }

    // The size the provider reports, or null when it does not say.
    private fun sizeOf(uri: Uri): Long? =
        context.contentResolver.query(uri, arrayOf(OpenableColumns.SIZE), null, null, null)?.use { cursor ->
            val column = cursor.getColumnIndex(OpenableColumns.SIZE)
            if (column >= 0 && cursor.moveToFirst() && !cursor.isNull(column)) cursor.getLong(column) else null
        }

    // For a provider that reports no size: stops reading as soon as the file is past the limit.
    private fun readAtMost(stream: InputStream): ByteArray {
        val out = ByteArrayOutputStream()
        val buffer = ByteArray(CHUNK_BYTES)
        var total = 0L
        while (true) {
            val count = stream.read(buffer)
            if (count < 0) return out.toByteArray()
            total += count
            if (total > MAX_BYTES) throw Failure.PhotoInvalid()
            out.write(buffer, 0, count)
        }
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

        /** The API's limit for one photo (ADR 0024). */
        const val MAX_BYTES = 10L * 1024 * 1024
        const val CHUNK_BYTES = 8 * 1024
    }
}
