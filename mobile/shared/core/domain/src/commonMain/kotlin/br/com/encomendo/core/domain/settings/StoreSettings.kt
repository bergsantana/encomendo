package br.com.encomendo.core.domain.settings

/**
 * StoreSettings is what the selected store turned on and its defaults, as far as the management screens
 * read them (`TenantSettings` in docs/api/openapi.yaml). Every role may read them; only the owner changes them,
 * on a screen this app does not have yet.
 */
data class StoreSettings(
    /** The `stock` sourcing mode ("Estoque") is available. */
    val stockEnabled: Boolean,
    /** The `in_house` sourcing mode ("Produção própria") is available. */
    val productionEnabled: Boolean,
    /** The store's default days of assembly, which a kit without its own uses. */
    val assemblyDays: Int,
    /** The store's default "Margem de segurança", in days. */
    val safetyMarginDays: Int,
)

/** SettingsRepository reads the selected store's settings. */
interface SettingsRepository {
    /** Returns the settings; throws `Failure.Forbidden` for a role that may not read them. */
    suspend fun settings(): StoreSettings
}
