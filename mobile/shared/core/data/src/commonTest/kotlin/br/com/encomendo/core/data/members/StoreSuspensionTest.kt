package br.com.encomendo.core.data.members

import br.com.encomendo.core.data.session.InMemoryStoreSuspensionNotice
import br.com.encomendo.core.data.session.createSessionApiClient
import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.domain.session.Role
import br.com.encomendo.core.domain.session.scope
import br.com.encomendo.core.network.ApiClientConfig
import br.com.encomendo.core.network.SessionTurns
import br.com.encomendo.core.network.members.MembersApi
import br.com.encomendo.core.security.SessionTokens
import br.com.encomendo.core.security.StoredSession
import br.com.encomendo.core.security.TokenStore
import br.com.encomendo.core.testing.AltaRpm
import br.com.encomendo.core.testing.FakeSecretCipher
import br.com.encomendo.core.testing.temporaryPreferencesDataStore
import br.com.encomendo.core.testing.testSession
import io.ktor.client.engine.mock.MockEngine
import io.ktor.client.engine.mock.MockRequestHandleScope
import io.ktor.client.engine.mock.respond
import io.ktor.client.request.HttpRequestData
import io.ktor.client.request.HttpResponseData
import io.ktor.http.HttpHeaders
import io.ktor.http.HttpStatusCode
import io.ktor.http.headersOf
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.job
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNotNull
import kotlin.test.assertNull

/**
 * ADR 0022 for every call a store's repositories make: a `tenant_suspended` answer raises the notice, the
 * caller gets `Failure.StoreSuspended`, and the session is renewed without the caller waiting for it.
 */
class StoreSuspensionTest {
    private val calls: List<Pair<String, suspend (Repositories) -> Unit>> =
        listOf(
            "members.page" to { it.members.page(cursor = null, limit = 100) },
            "members.changeRole" to { it.members.changeRole(userId = "u-1", role = Role.Viewer) },
            "members.remove" to { it.members.remove(userId = "u-1") },
            "invitations.page" to { it.invitations.page(cursor = null, limit = 100) },
            "invitations.invite" to { it.invitations.invite(email = "carla@altarpm.com.br", role = Role.Viewer) },
            "invitations.revoke" to { it.invitations.revoke(invitationId = "i-1") },
        )

    @Test
    fun everyCallInTheStoreRaisesTheNoticeAndRenewsTheSession() =
        runTest {
            for ((name, call) in calls) {
                val wired = wired { request -> if (isRefresh(request)) json(SUSPENDED_SESSION) else suspended() }

                assertFailsWith<Failure.StoreSuspended>(name) { call(wired.repositories) }
                wired.renewalDone()

                assertEquals(AltaRpm.id, wired.notice.raisedFor.value, name)
                assertEquals(1, wired.paths.count { it == "/auth/refresh" }, name)
                val session = assertNotNull(wired.store.read(), name).session
                assertNull(session.tenant, name)
                assertEquals(listOf(true), session.memberships.map { it.suspended }, name)
            }
        }

    @Test
    fun theCallerDoesNotWaitForTheRenewal() =
        runTest {
            // The failure comes back at once; the renewal (and its retries) goes on without the caller.
            val refreshing = CompletableDeferred<Unit>()
            val mayAnswer = CompletableDeferred<Unit>()
            val wired =
                wired { request ->
                    if (isRefresh(request)) {
                        refreshing.complete(Unit)
                        mayAnswer.await()
                        json(SUSPENDED_SESSION)
                    } else {
                        suspended()
                    }
                }

            assertFailsWith<Failure.StoreSuspended> { wired.repositories.members.page(cursor = null, limit = 100) }
            refreshing.await()
            assertEquals(AltaRpm.id, wired.notice.raisedFor.value)
            assertNotNull(
                wired.store
                    .read()
                    ?.session
                    ?.tenant,
                "still in the store until the renewal answers",
            )

            mayAnswer.complete(Unit)
            wired.renewalDone()
            assertNull(
                wired.store
                    .read()
                    ?.session
                    ?.tenant,
            )
        }

    @Test
    fun aRenewalTheServerRefusesEndsTheSessionAndTheCallerStillHearsSuspended() =
        runTest {
            // A 401 on the refresh token ends the session (ADR 0012); the caller's failure is not replaced.
            val wired =
                wired { request ->
                    if (isRefresh(request)) {
                        respond(
                            """{"title":"Sessão expirada","status":401,"code":"unauthenticated"}""",
                            HttpStatusCode.Unauthorized,
                            headersOf(HttpHeaders.ContentType, "application/problem+json"),
                        )
                    } else {
                        suspended()
                    }
                }

            assertFailsWith<Failure.StoreSuspended> { wired.repositories.members.page(cursor = null, limit = 100) }
            wired.renewalDone()

            assertNull(wired.store.read())
        }

    @Test
    fun aRefusedRoleRenewsNothingAndRaisesNothing() =
        runTest {
            val wired =
                wired {
                    respond(
                        """{"title":"Recusado","status":403,"code":"forbidden"}""",
                        HttpStatusCode.Forbidden,
                        headersOf(HttpHeaders.ContentType, "application/problem+json"),
                    )
                }

            assertFailsWith<Failure.Forbidden> { wired.repositories.members.remove(userId = "u-1") }
            wired.renewalDone()

            assertEquals(listOf("/api/v1/admin/members/u-1"), wired.paths)
            assertNull(wired.notice.raisedFor.value)
        }

    private class Repositories(
        val members: ApiMembersRepository,
        val invitations: ApiInvitationsRepository,
    )

    private class Wired(
        val repositories: Repositories,
        val store: TokenStore,
        val notice: InMemoryStoreSuspensionNotice,
        val paths: List<String>,
        private val renewals: CoroutineScope,
    ) {
        /** Waits for the renewals the calls started. */
        suspend fun renewalDone() =
            renewals.coroutineContext.job.children
                .toList()
                .forEach { it.join() }
    }

    private suspend fun TestScope.wired(
        handler: suspend MockRequestHandleScope.(HttpRequestData) -> HttpResponseData,
    ): Wired {
        val store =
            TokenStore(
                temporaryPreferencesDataStore(backgroundScope),
                FakeSecretCipher(),
                StandardTestDispatcher(testScheduler),
            )
        store.write(StoredSession(SessionTokens("access", "refresh"), testSession()))
        val turns = SessionTurns()
        // Only screens of the stored session's own scope may send its token (ADR 0018).
        turns.screensServe(testSession().scope)
        val paths = mutableListOf<String>()
        val engine =
            MockEngine { request ->
                paths += request.url.encodedPath
                handler(request)
            }
        val client = createSessionApiClient(engine, ApiClientConfig(baseUrl = "https://api.test"), store, turns)
        val notice = InMemoryStoreSuspensionNotice()
        val renewals = CoroutineScope(SupervisorJob() + Dispatchers.Default)
        val calls = storeCalls(store, client, turns, notice, renewals)
        val api = MembersApi(client)
        return Wired(
            Repositories(ApiMembersRepository(api, calls), ApiInvitationsRepository(api, calls)),
            store,
            notice,
            paths,
            renewals,
        )
    }

    private fun isRefresh(request: HttpRequestData) = request.url.encodedPath == "/auth/refresh"

    private fun MockRequestHandleScope.json(body: String) =
        respond(body, HttpStatusCode.OK, headersOf(HttpHeaders.ContentType, "application/json"))

    private fun MockRequestHandleScope.suspended() =
        respond(
            """{"title":"Esta loja está suspensa.","status":403,"code":"tenant_suspended"}""",
            HttpStatusCode.Forbidden,
            headersOf(HttpHeaders.ContentType, "application/problem+json"),
        )

    private companion object {
        /** What `/auth/refresh` answers once the platform suspended the store: no store selected, and it marked. */
        const val SUSPENDED_SESSION = """
            {"accessToken": "access-renewed", "tokenType": "Bearer", "expiresIn": 900, "refreshToken": "refresh-renewed",
             "user": {"id": "0190f3c4-0000-7000-8000-0000000000aa", "name": "Ana Souza", "email": "ana@altarpm.com.br"},
             "permissions": [],
             "memberships": [{"tenant": {"id": "0190f3c4-0000-7000-8000-000000000001", "name": "Alta RPM",
                                         "slug": "alta-rpm"}, "role": "owner", "status": "suspended"}]}
        """
    }
}
