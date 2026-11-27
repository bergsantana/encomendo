package br.com.encomendo.core.data.settings

import br.com.encomendo.core.data.failure.StoreCalls
import br.com.encomendo.core.domain.settings.SettingsRepository
import br.com.encomendo.core.domain.settings.StoreSettings
import br.com.encomendo.core.network.settings.SettingsApi

/** ApiSettingsRepository reads the store's settings through [api], in the session's store. */
class ApiSettingsRepository(
    private val api: SettingsApi,
    private val calls: StoreCalls,
) : SettingsRepository {
    override suspend fun settings(): StoreSettings =
        calls.inStore {
            val dto = api.get()
            StoreSettings(dto.stockEnabled, dto.productionEnabled, dto.assemblyDays, dto.safetyMarginDays)
        }
}
