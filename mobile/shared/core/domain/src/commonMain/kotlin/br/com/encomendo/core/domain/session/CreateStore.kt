package br.com.encomendo.core.domain.session

/**
 * CreateStore creates a store owned by the signed-in user and opens it. The
 * name is trimmed and the address (its link, `{slug}.encomendo.com.br`)
 * trimmed and lower-cased; the API still checks both.
 *
 * Keep one instance per form: when the store was created but opening it
 * failed, a retry with the same link only opens it, because creating it
 * again would say its own link is taken.
 */
class CreateStore(
    private val tenancy: TenancyRepository,
) {
    private var created: TenantSummary? = null

    /** Creates the store, opens it and returns the new session. Failures are thrown as `Failure`. */
    suspend operator fun invoke(
        name: String,
        slug: String,
    ): Session {
        val address = slug.trim().lowercase()
        val store = created?.takeIf { it.slug == address } ?: tenancy.createStore(name.trim(), address)
        created = store
        val session = tenancy.switchTo(store.id)
        created = null
        return session
    }
}
