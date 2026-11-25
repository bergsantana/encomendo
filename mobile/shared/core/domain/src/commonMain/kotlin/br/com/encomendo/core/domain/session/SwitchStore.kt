package br.com.encomendo.core.domain.session

/**
 * SwitchStore opens another store the user belongs to; the app then shows that
 * store's screens from scratch. Failures are thrown as `Failure`.
 */
class SwitchStore(
    private val tenancy: TenancyRepository,
) {
    /** Moves the session to [tenantId] and returns it. */
    suspend operator fun invoke(tenantId: String): Session = tenancy.switchTo(tenantId)
}
