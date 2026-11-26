package br.com.encomendo.core.domain.members

import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.domain.paging.Page
import br.com.encomendo.core.domain.session.Role
import br.com.encomendo.core.domain.session.Session
import br.com.encomendo.core.domain.session.SessionRepository
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertFalse
import kotlin.time.Instant

class InvitationsTest {
    @Test
    fun invitationsAreReadAHundredAtATime() =
        runTest {
            val invitations = RecordingInvitations()

            ListInvitations(invitations, RecordingSessions())(cursor = "c2")

            assertEquals(listOf<Pair<String?, Int>>("c2" to 100), invitations.pages)
        }

    @Test
    fun anInvitationGoesToTheTrimmedEmail() =
        runTest {
            val invitations = RecordingInvitations()

            InviteMember(invitations, RecordingSessions())(email = "  bruno@altarpm.com.br ", role = Role.Staff)

            assertEquals(listOf("bruno@altarpm.com.br" to Role.Staff), invitations.sent)
        }

    @Test
    fun nobodyIsInvitedAsOwner() =
        runTest {
            assertFailsWith<IllegalArgumentException> {
                InviteMember(
                    RecordingInvitations(),
                    RecordingSessions(),
                )(email = "bruno@altarpm.com.br", role = Role.Owner)
            }
        }

    @Test
    fun cancellingSendsTheInvitationsId() =
        runTest {
            val invitations = RecordingInvitations()

            RevokeInvitation(invitations, RecordingSessions())(BRUNO)

            assertEquals(listOf(BRUNO.id), invitations.revoked)
        }

    @Test
    fun aRefusalRenewsTheSessionAndStillFails() =
        runTest {
            // As for the team: a role lost elsewhere shows at once, so Equipe can leave.
            val invitations = RecordingInvitations(answer = { throw Failure.Forbidden() })
            val sessions = RecordingSessions()

            assertFailsWith<Failure.Forbidden> { ListInvitations(invitations, sessions)() }
            assertFailsWith<Failure.Forbidden> {
                InviteMember(
                    invitations,
                    sessions,
                )("bruno@altarpm.com.br", Role.Staff)
            }
            assertFailsWith<Failure.Forbidden> { RevokeInvitation(invitations, sessions)(BRUNO) }

            assertEquals(3, sessions.renewals)
        }

    @Test
    fun anUnsentEmailLeavesTheSessionAlone() =
        runTest {
            val invitations = RecordingInvitations(answer = { throw Failure.EmailNotSent() })
            val sessions = RecordingSessions()

            assertFailsWith<Failure.EmailNotSent> {
                InviteMember(
                    invitations,
                    sessions,
                )("bruno@altarpm.com.br", Role.Staff)
            }

            assertEquals(0, sessions.renewals)
        }

    @Test
    fun anInvitationsRowIsItsEmailWhateverTheCase() {
        // "Reenviar" can give the invitation a new id, and the API compares emails without case.
        assertEquals(BRUNO.key, BRUNO.copy(id = "another", email = "Bruno@AltaRPM.com.br").key)
    }

    @Test
    fun anInvitationInALogCarriesNoEmail() {
        assertFalse("bruno@" in BRUNO.toString(), BRUNO.toString())
    }

    private class RecordingInvitations(
        val answer: suspend () -> Unit = {},
    ) : InvitationsRepository {
        val pages = mutableListOf<Pair<String?, Int>>()
        val sent = mutableListOf<Pair<String, Role>>()
        val revoked = mutableListOf<String>()

        override suspend fun page(
            cursor: String?,
            limit: Int,
        ): Page<Invitation> {
            pages += cursor to limit
            answer()
            return Page(items = listOf(BRUNO), nextCursor = null)
        }

        override suspend fun invite(
            email: String,
            role: Role,
        ): Invitation {
            answer()
            sent += email to role
            return BRUNO.copy(email = email, role = role)
        }

        override suspend fun revoke(invitationId: String) {
            answer()
            revoked += invitationId
        }
    }

    private class RecordingSessions : SessionRepository {
        var renewals = 0

        override val session = MutableStateFlow<Session?>(null)

        override suspend fun renew() {
            renewals++
        }
    }

    private companion object {
        val BRUNO =
            Invitation(
                id = "0190f3c4-0000-7000-8000-0000000000e1",
                email = "bruno@altarpm.com.br",
                role = Role.Staff,
                expiresAt = Instant.parse("2026-12-02T12:00:00Z"),
            )
    }
}
