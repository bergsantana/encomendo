package br.com.encomendo.core.data.session

import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals

class DeviceDataCleanersTest {
    @Test
    fun everyCleanerRunsInOrder() =
        runTest {
            val ran = mutableListOf<String>()
            val catalog = DeviceDataCleaner { ran += "catalog" }
            val images = DeviceDataCleaner { ran += "images" }

            DeviceDataCleaners(listOf(catalog, images)).clear()

            assertEquals(listOf("catalog", "images"), ran)
        }
}
