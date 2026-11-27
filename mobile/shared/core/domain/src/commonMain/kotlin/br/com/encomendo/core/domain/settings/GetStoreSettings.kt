package br.com.encomendo.core.domain.settings

/** GetStoreSettings reads what the store turned on, such as the "Estoque" and "Produção própria" origins. */
class GetStoreSettings(
    private val settings: SettingsRepository,
) {
    /** Returns the settings; throws `Failure.Forbidden` for a role that may not read them. */
    suspend operator fun invoke(): StoreSettings = settings.settings()
}
