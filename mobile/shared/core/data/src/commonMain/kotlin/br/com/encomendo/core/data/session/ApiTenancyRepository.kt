package br.com.encomendo.core.data.session

import br.com.encomendo.core.data.failure.asFailures
import br.com.encomendo.core.domain.session.Session
import br.com.encomendo.core.domain.session.TenancyRepository
import br.com.encomendo.core.domain.session.TenantSummary
import br.com.encomendo.core.network.ApiError
import br.com.encomendo.core.network.tenancy.TenancyApi

/**
 * ApiTenancyRepository creates stores and switches the session between them.
 * The switch spends the refresh token, so [keeper] makes it in its turn
 * (ADR 0010, ADR 0013). The new store's token goes out only once the app's
 * screens serve it (ADR 0018).
 */
class ApiTenancyRepository(
    private val tenancy: TenancyApi,
    private val keeper: SessionKeeper,
) : TenancyRepository {
    override suspend fun switchTo(tenantId: String): Session =
        asFailures {
            try {
                keeper.switchTo(tenantId).session
            } catch (error: ApiError) {
                // The user probably lost that store, or it was suspended (`tenant_suspended` is a 403 too):
                // renew once, so the picker's list drops or marks it, as the web does. The 403 itself left
                // the refresh token and the store as they were (ADR 0013, ADR 0022).
                if (error.status == FORBIDDEN) keeper.renew()
                throw error
            }
        }

    override suspend fun createStore(
        name: String,
        slug: String,
    ): TenantSummary =
        asFailures {
            val tenant = tenancy.createTenant(name = name, slug = slug)
            TenantSummary(id = tenant.id, name = tenant.name, slug = tenant.slug)
        }

    private companion object {
        const val FORBIDDEN = 403
    }
}
