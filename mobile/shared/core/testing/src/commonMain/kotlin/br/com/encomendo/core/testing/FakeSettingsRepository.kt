package br.com.encomendo.core.testing

import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.domain.settings.SettingsRepository
import br.com.encomendo.core.domain.settings.StoreSettings

/**
 * FakeSettingsRepository answers with the settings a test sets, or throws [failure] when it is set. By
 * default the store has stock and production on.
 */
class FakeSettingsRepository(
    /** What the store has turned on. */
    var current: StoreSettings =
        StoreSettings(stockEnabled = true, productionEnabled = true, assemblyDays = 3, safetyMarginDays = 2),
) : SettingsRepository {
    /** When set, every read throws it. */
    var failure: Failure? = null

    override suspend fun settings(): StoreSettings {
        failure?.let { throw it }
        return current
    }
}
