package br.com.encomendo.core.data.members

import br.com.encomendo.core.data.session.createSessionApiClient
import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.domain.members.Invitation
import br.com.encomendo.core.domain.session.Role
import br.com.encomendo.core.network.ApiClientConfig
import br.com.encomendo.core.network.SessionTurns
import br.com.encomendo.core.network.members.MembersApi
import br.com.encomendo.core.security.SessionTokens
import br.com.encomendo.core.security.StoredSession
import br.com.encomendo.core.security.TokenStore
import br.com.encomendo.core.testing.FakeSecretCipher
import br.com.encomendo.core.testing.temporaryPreferencesDataStore
import br.com.encomendo.core.testing.testSession
import io.ktor.client.engine.mock.MockEngine
import io.ktor.client.engine.mock.MockRequestHandleScope
import io.ktor.client.engine.mock.MockRequestHandler
import io.ktor.client.engine.mock.respond
import io.ktor.http.HttpHeaders
import io.ktor.http.HttpStatusCode
import io.ktor.http.headersOf
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.time.Instant

class ApiInvitationsRepositoryTest {
    @Test
    fun readsThePendingInvitations() =
        runTest {
            val invitations = repository { json("""{"items": [$CARLA_JSON], "nextCursor": "i2"}""") }

            val page = invitations.page(cursor = null, limit = 100)

            assertEquals(listOf(CARLA), page.items)
            assertEquals("i2", page.nextCursor)
        }

    @Test
    fun anInvitationComesBackAsSaved() =
        runTest {
            val invitations = repository { json(CARLA_JSON, HttpStatusCode.Created) }

            assertEquals(CARLA, invitations.invite(email = CARLA.email, role = Role.Viewer))
        }

    @Test
    fun someoneOnTheTeamAlreadyIsAlreadyAMember() =
        runTest {
            val invitations = repository { problem(409, "already_member") }

            assertFailsWith<Failure.AlreadyMember> { invitations.invite(email = CARLA.email, role = Role.Viewer) }
        }

    @Test
    fun anEmailThatDidNotGoOutIsNotAServerFailure() =
        runTest {
            // A 503, but the invitation was saved: the screen says so and offers "Reenviar".
            val invitations = repository { problem(503, "email_unavailable") }

            assertFailsWith<Failure.EmailNotSent> { invitations.invite(email = CARLA.email, role = Role.Viewer) }
        }

    @Test
    fun anInvitationNoLongerPendingIsNotFound() =
        runTest {
            val invitations = repository { problem(404, "not_found") }

            assertFailsWith<Failure.NotFound> { invitations.revoke(invitationId = CARLA.id) }
        }

    @Test
    fun anExpiryThatIsNotADateIsAnAnswerTheAppCannotUse() =
        runTest {
            val invitations =
                repository { json("""{"items": [${CARLA_JSON.replace("2026-12-02T12:00:00Z", "amanhã")}]}""") }

            val failure = assertFailsWith<Failure.Unexpected> { invitations.page(cursor = null, limit = 100) }

            assertEquals("unknown", failure.code)
        }

    @Test
    fun anInvitationForARoleNobodyCanBeInvitedWithIsAnAnswerTheAppCannotUse() =
        runTest {
            // The contract's InvitableRole; "Reenviar" sends the role back as it came, so it must be one.
            val invitations = repository { json("""{"items": [${CARLA_JSON.replace("\"viewer\"", "\"owner\"")}]}""") }

            val failure = assertFailsWith<Failure.Unexpected> { invitations.page(cursor = null, limit = 100) }

            assertEquals("unknown", failure.code)
        }

    private fun MockRequestHandleScope.json(
        body: String,
        status: HttpStatusCode = HttpStatusCode.OK,
    ) = respond(body, status, headersOf(HttpHeaders.ContentType, "application/json"))

    private fun MockRequestHandleScope.problem(
        status: Int,
        code: String,
    ) = respond(
        """{"title":"Recusado","status":$status,"code":"$code"}""",
        HttpStatusCode.fromValue(status),
        headersOf(HttpHeaders.ContentType, "application/problem+json"),
    )

    private suspend fun TestScope.repository(handler: MockRequestHandler): ApiInvitationsRepository {
        val store =
            TokenStore(
                temporaryPreferencesDataStore(backgroundScope),
                FakeSecretCipher(),
                StandardTestDispatcher(testScheduler),
            )
        store.write(StoredSession(SessionTokens("access", "refresh"), testSession()))
        val turns = SessionTurns()
        val client =
            createSessionApiClient(MockEngine(handler), ApiClientConfig(baseUrl = "https://api.test"), store, turns)
        return ApiInvitationsRepository(MembersApi(client), storeCalls(store, client, turns))
    }

    private companion object {
        val CARLA =
            Invitation(
                id = "0190f3c4-0000-7000-8000-0000000000e1",
                email = "carla@altarpm.com.br",
                role = Role.Viewer,
                expiresAt = Instant.parse("2026-12-02T12:00:00Z"),
            )

        const val CARLA_JSON = """
            {"id": "0190f3c4-0000-7000-8000-0000000000e1", "email": "carla@altarpm.com.br", "role": "viewer",
             "invitedAt": "2026-11-25T12:00:00Z", "expiresAt": "2026-12-02T12:00:00Z"}
        """
    }
}
