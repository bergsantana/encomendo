package br.com.encomendo.core.testing

import br.com.encomendo.core.domain.session.Session
import br.com.encomendo.core.domain.session.TenancyRepository
import br.com.encomendo.core.domain.session.TenantSummary

/**
 * FakeTenancyRepository records every switch and store creation and answers
 * with [switchAnswer] and [createAnswer], which return or throw a `Failure`.
 * A test that needs a call to hang makes it wait on a `CompletableDeferred`.
 */
class FakeTenancyRepository(
    var switchAnswer: suspend (tenantId: String) -> Session = { testSession() },
    var createAnswer: suspend (name: String, slug: String) -> TenantSummary = { name, slug ->
        TenantSummary(id = "0190f3c4-0000-7000-8000-000000000009", name = name, slug = slug)
    },
) : TenancyRepository {
    private val switched = mutableListOf<String>()
    private val createdStores = mutableListOf<Pair<String, String>>()

    /** The store ids switched to so far, in order. */
    val switches: List<String> get() = switched.toList()

    /** The (name, slug) pairs created so far, in order. */
    val created: List<Pair<String, String>> get() = createdStores.toList()

    override suspend fun switchTo(tenantId: String): Session {
        switched += tenantId
        return switchAnswer(tenantId)
    }

    override suspend fun createStore(
        name: String,
        slug: String,
    ): TenantSummary {
        createdStores += name to slug
        return createAnswer(name, slug)
    }
}
