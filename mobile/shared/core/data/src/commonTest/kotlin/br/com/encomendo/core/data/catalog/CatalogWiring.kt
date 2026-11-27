package br.com.encomendo.core.data.catalog

import br.com.encomendo.core.data.members.storeCalls
import br.com.encomendo.core.data.session.createSessionApiClient
import br.com.encomendo.core.domain.session.Role
import br.com.encomendo.core.domain.session.Session
import br.com.encomendo.core.domain.session.scope
import br.com.encomendo.core.network.ApiClientConfig
import br.com.encomendo.core.network.SessionTurns
import br.com.encomendo.core.network.catalog.CatalogApis
import br.com.encomendo.core.network.catalog.PhotoStorageApi
import br.com.encomendo.core.network.catalog.SuppliersApi
import br.com.encomendo.core.network.catalog.createStorageClient
import br.com.encomendo.core.security.SessionTokens
import br.com.encomendo.core.security.StoredSession
import br.com.encomendo.core.security.TokenStore
import br.com.encomendo.core.testing.FakeSecretCipher
import br.com.encomendo.core.testing.FakeSessionRepository
import br.com.encomendo.core.testing.temporaryPreferencesDataStore
import br.com.encomendo.core.testing.testSession
import io.ktor.client.engine.mock.MockEngine
import io.ktor.client.engine.mock.MockRequestHandleScope
import io.ktor.client.engine.mock.respond
import io.ktor.client.engine.mock.toByteArray
import io.ktor.client.request.HttpRequestData
import io.ktor.client.request.HttpResponseData
import io.ktor.http.HttpHeaders
import io.ktor.http.HttpStatusCode
import io.ktor.http.headersOf
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestScope
import kotlinx.io.IOException

typealias Handler = suspend MockRequestHandleScope.(HttpRequestData) -> HttpResponseData

/**
 * The catalog and supplier repositories wired over mock engines: one for the API (signed in as [session]) and one
 * for the storage host. A test changes [api] or [storage] to change what the servers answer.
 */
class CatalogWiring(
    val catalog: ApiCatalogRepository,
    val suppliers: ApiSupplierRepository,
    val cache: InMemoryCatalogCacheDao,
    val clock: FixedClock,
    val sessions: FakeSessionRepository,
    private val state: HandlerState,
) {
    /** What the API answers; replace it to go offline or to fail. */
    var api: Handler by state::api

    /** What the storage host answers. */
    var storage: Handler by state::storage

    /** Every request the API client sent, in order. */
    val apiRequests: List<HttpRequestData> get() = state.apiRequests

    /** Every request the storage client sent, in order. */
    val storageRequests: List<HttpRequestData> get() = state.storageRequests
}

/** The mutable handlers behind [CatalogWiring]. */
class HandlerState {
    var api: Handler = { respondJson("{}") }
    var storage: Handler = { respond("", HttpStatusCode.OK) }
    val apiRequests = mutableListOf<HttpRequestData>()
    val storageRequests = mutableListOf<HttpRequestData>()
}

fun MockRequestHandleScope.respondJson(
    body: String,
    status: HttpStatusCode = HttpStatusCode.OK,
): HttpResponseData = respond(body, status, headersOf(HttpHeaders.ContentType, "application/json"))

fun MockRequestHandleScope.respondProblem(
    status: Int,
    code: String,
    extra: String = "",
): HttpResponseData =
    respond(
        """{"title":"Recusado","status":$status,"code":"$code"$extra}""",
        HttpStatusCode.fromValue(status),
        headersOf(HttpHeaders.ContentType, "application/problem+json"),
    )

/** Answers as a phone with no signal does. */
@Suppress("UnusedReceiverParameter") // Reads as a response next to the others in a handler.
fun MockRequestHandleScope.noSignal(): Nothing = throw IOException("no signal")

suspend fun HttpRequestData.bodyText(): String = body.toByteArray().decodeToString()

/** Wires the repositories for a user with [role] in the usual test store. */
suspend fun TestScope.catalogWiring(role: Role = Role.Owner): CatalogWiring {
    val session: Session = testSession().copy(role = role)
    val store =
        TokenStore(
            temporaryPreferencesDataStore(backgroundScope),
            FakeSecretCipher(),
            StandardTestDispatcher(testScheduler),
        )
    store.write(StoredSession(SessionTokens("access", "refresh"), session))
    val turns = SessionTurns()
    // Only screens of the stored session's own scope may send its token (ADR 0018).
    turns.screensServe(session.scope)
    val state = HandlerState()
    val client =
        createSessionApiClient(
            MockEngine { request ->
                state.apiRequests += request
                state.api(this, request)
            },
            ApiClientConfig(baseUrl = "https://api.test"),
            store,
            turns,
        )
    val storageClient =
        createStorageClient(
            MockEngine { request ->
                state.storageRequests += request
                state.storage(this, request)
            },
        )
    val calls = storeCalls(store, client, turns)
    val cache = InMemoryCatalogCacheDao()
    val clock = FixedClock()
    val sessions = FakeSessionRepository(session)
    return CatalogWiring(
        catalog =
            ApiCatalogRepository(CatalogApis(client), PhotoStorageApi(storageClient), calls, cache, sessions, clock),
        suppliers = ApiSupplierRepository(SuppliersApi(client), calls, cache, sessions, clock),
        cache = cache,
        clock = clock,
        sessions = sessions,
        state = state,
    )
}
