package br.com.encomendo.core.data.session

import br.com.encomendo.core.network.ApiClientConfig
import br.com.encomendo.core.network.BearerCredentials
import br.com.encomendo.core.network.TokenRefresher
import br.com.encomendo.core.network.TokenStorage
import br.com.encomendo.core.network.auth.AuthApi
import br.com.encomendo.core.network.createApiClient
import br.com.encomendo.core.security.StoredSession
import br.com.encomendo.core.security.TokenStore
import io.ktor.client.HttpClient
import io.ktor.client.engine.HttpClientEngine

/**
 * Creates the app's one API client, signed in through [store]: requests carry
 * the stored access token, and a refresh replaces the tokens and the session's
 * store, role and permissions together. Keep one per process, so refreshes
 * never run twice at once (docs/api/openapi.yaml, "Sessions").
 */
fun createSessionApiClient(
    engine: HttpClientEngine,
    config: ApiClientConfig,
    store: TokenStore,
): HttpClient = createApiClient(engine, config, TokenStoreStorage(store), SessionRefresher)

/** A stored session as the API client sees it: its tokens. The rest travels along untouched. */
internal class StoredCredentials(
    val stored: StoredSession,
) : BearerCredentials {
    override val accessToken: String get() = stored.tokens.accessToken
    override val refreshToken: String get() = stored.tokens.refreshToken
}

/** Backs the API client's [TokenStorage] with the device's [TokenStore]. */
internal class TokenStoreStorage(
    private val store: TokenStore,
) : TokenStorage<StoredCredentials> {
    override suspend fun read(): StoredCredentials? = store.read()?.let(::StoredCredentials)

    override suspend fun replace(
        expected: StoredCredentials,
        replacement: StoredCredentials?,
    ): Boolean = store.replace(expected = expected.stored, replacement = replacement?.stored)
}

/** Refreshes through `/auth/refresh`, keeping the whole new session, not just its tokens. */
internal object SessionRefresher : TokenRefresher<StoredCredentials> {
    override suspend fun refresh(
        client: HttpClient,
        current: StoredCredentials,
    ): StoredCredentials = StoredCredentials(AuthApi(client).refresh(current.refreshToken).toStoredSession())
}
