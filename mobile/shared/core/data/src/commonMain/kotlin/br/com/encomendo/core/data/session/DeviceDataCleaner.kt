package br.com.encomendo.core.data.session

/**
 * DeviceDataCleaner forgets what the device kept for the person who is leaving, other than their session: photos in
 * the image caches, which signed URLs let anyone holding the phone see (ADR 0016, ADR 0026). Sair calls it right
 * after the session is cleared. It must not throw: leaving never fails because a cache could not be emptied.
 */
fun interface DeviceDataCleaner {
    /** Empties what the device cached, as far as it can. */
    suspend fun clear()
}
