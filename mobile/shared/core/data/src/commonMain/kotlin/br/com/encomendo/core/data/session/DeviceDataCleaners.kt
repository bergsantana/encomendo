package br.com.encomendo.core.data.session

/**
 * DeviceDataCleaners runs each of [cleaners] in turn, so Sair empties everything the device kept (the catalog cache,
 * the image caches) with one call. Each cleaner already swallows its own failures, so one that cannot empty its cache
 * never leaves the next one's data on the phone.
 */
class DeviceDataCleaners(
    private val cleaners: List<DeviceDataCleaner>,
) : DeviceDataCleaner {
    override suspend fun clear() = cleaners.forEach { it.clear() }
}
