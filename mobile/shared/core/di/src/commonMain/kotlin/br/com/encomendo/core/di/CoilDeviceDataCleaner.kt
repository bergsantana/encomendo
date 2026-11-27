package br.com.encomendo.core.di

import br.com.encomendo.core.data.session.DeviceDataCleaner
import coil3.ImageLoader
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.withContext
import okio.IOException

/**
 * CoilDeviceDataCleaner empties the photo thumbnails' memory and disk caches when someone leaves (ADR 0016). The disk
 * cache is blocking file work, so it runs on [io]. A store switch keeps both caches: a signed URL names one object of
 * one store, so the next store's screens never ask for the old store's pictures.
 */
internal class CoilDeviceDataCleaner(
    private val loader: ImageLoader,
    private val io: CoroutineDispatcher,
) : DeviceDataCleaner {
    override suspend fun clear() {
        loader.memoryCache?.clear()
        try {
            withContext(io) { loader.diskCache?.clear() }
        } catch (_: IOException) {
            // A cache that cannot be emptied must not stop someone from leaving: its files are signed URLs' pictures.
        }
    }
}
