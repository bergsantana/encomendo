package br.com.encomendo.core.network.settings

import br.com.encomendo.core.network.decoded
import io.ktor.client.HttpClient
import io.ktor.client.request.get
import kotlinx.serialization.Serializable

/**
 * SettingsApi reads the selected store's settings (docs/api/openapi.yaml, `getSettings`). The call carries the
 * session's access token and needs `settings:read`, which every role has.
 */
class SettingsApi(
    private val client: HttpClient,
) {
    /** Returns the store's settings. */
    suspend fun get(): SettingsDto = client.get("/api/v1/admin/settings").decoded()
}

/** SettingsDto is the part of `TenantSettings` the management screens read. */
@Serializable
data class SettingsDto(
    val stockEnabled: Boolean,
    val productionEnabled: Boolean,
    val assemblyDays: Int,
    val safetyMarginDays: Int,
)
