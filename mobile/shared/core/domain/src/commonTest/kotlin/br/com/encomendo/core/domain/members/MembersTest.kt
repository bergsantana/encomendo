package br.com.encomendo.core.domain.members

import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.domain.paging.Page
import br.com.encomendo.core.domain.session.GrantedPermissions
import br.com.encomendo.core.domain.session.Role
import br.com.encomendo.core.domain.session.Session
import br.com.encomendo.core.domain.session.SessionRepository
import br.com.encomendo.core.domain.session.TenantSummary
import br.com.encomendo.core.domain.session.User
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertFalse
import kotlin.test.assertTrue

class MembersTest {
    @Test
    fun theTeamIsReadAHundredAtATime() =
        runTest {
            // Teams are small: one page is usually the whole team, as on the web.
            val members = RecordingMembers()

            ListMembers(members, RecordingSessions())(cursor = "c2")

            assertEquals(listOf<Pair<String?, Int>>("c2" to 100), members.pages)
        }

    @Test
    fun changingSomeoneElsesRoleLeavesTheSessionAlone() =
        runTest {
            val sessions = RecordingSessions()

            val changed = ChangeMemberRole(RecordingMembers(), sessions)(BRUNO, Role.Viewer)

            assertEquals(Role.Viewer, changed.role)
            assertEquals(0, sessions.renewals)
        }

    @Test
    fun changingOnesOwnRoleRenewsTheSession() =
        runTest {
            // The permissions shown follow at once (the web's session.refresh()): an admin made staff loses Equipe.
            val sessions = RecordingSessions()

            ChangeMemberRole(RecordingMembers(), sessions)(ANA_AS_ADMIN, Role.Staff)

            assertEquals(1, sessions.renewals)
        }

    @Test
    fun removingOneselfRenewsTheSession() =
        runTest {
            // The renewed session no longer has this store (ADR 0010), so the app leaves it.
            val sessions = RecordingSessions()
            val members = RecordingMembers()

            RemoveMember(members, sessions)(ANA_AS_ADMIN)

            assertEquals(listOf(ANA_AS_ADMIN.userId), members.removed)
            assertEquals(1, sessions.renewals)
        }

    @Test
    fun removingSomeoneElseLeavesTheSessionAlone() =
        runTest {
            val sessions = RecordingSessions()

            RemoveMember(RecordingMembers(), sessions)(BRUNO)

            assertEquals(0, sessions.renewals)
        }

    @Test
    fun aRefusalRenewsTheSessionAndStillFails() =
        runTest {
            // A role lost elsewhere shows at once, so the screen can leave (design document, "Equipe").
            val refused = Failure.Forbidden(title = "Sem permissão")
            val members = RecordingMembers(answer = { throw refused })

            val sessions = RecordingSessions()
            assertFailsWith<Failure.Forbidden> { ListMembers(members, sessions)() }
            assertFailsWith<Failure.Forbidden> { ChangeMemberRole(members, sessions)(BRUNO, Role.Admin) }
            assertFailsWith<Failure.Forbidden> { RemoveMember(members, sessions)(BRUNO) }

            assertEquals(3, sessions.renewals)
        }

    @Test
    fun anyOtherFailureLeavesTheSessionAlone() =
        runTest {
            val members = RecordingMembers(answer = { throw Failure.NotFound() })
            val sessions = RecordingSessions()

            assertFailsWith<Failure.NotFound> { ChangeMemberRole(members, sessions)(ANA_AS_ADMIN, Role.Staff) }

            assertEquals(0, sessions.renewals, "nothing changed, so there is nothing to catch up on")
        }

    @Test
    fun nobodyIsMadeOwnerHere() =
        runTest {
            // A store has one owner, and the contract's InvitableRole leaves owner out.
            assertFailsWith<IllegalArgumentException> {
                ChangeMemberRole(RecordingMembers(), RecordingSessions())(BRUNO, Role.Owner)
            }
        }

    @Test
    fun theOwnersRowHasNothingToChange() {
        assertFalse(ANA_AS_ADMIN.copy(role = Role.Owner).canChange)
        assertTrue(ANA_AS_ADMIN.canChange)
        assertEquals(listOf(Role.Admin, Role.Staff, Role.Viewer), Role.Invitable)
    }

    @Test
    fun aMemberInALogCarriesNoPersonalData() {
        val printed = BRUNO.toString()

        assertFalse("Bruno" in printed || "bruno@" in printed, printed)
    }

    private class RecordingMembers(
        val answer: suspend () -> Unit = {},
    ) : MembersRepository {
        val pages = mutableListOf<Pair<String?, Int>>()
        val removed = mutableListOf<String>()

        override suspend fun page(
            cursor: String?,
            limit: Int,
        ): Page<Member> {
            pages += cursor to limit
            answer()
            return Page(items = listOf(ANA_AS_ADMIN, BRUNO), nextCursor = null)
        }

        override suspend fun changeRole(
            userId: String,
            role: Role,
        ): Member {
            answer()
            return listOf(ANA_AS_ADMIN, BRUNO).first { it.userId == userId }.copy(role = role)
        }

        override suspend fun remove(userId: String) {
            answer()
            removed += userId
        }
    }

    private class RecordingSessions : SessionRepository {
        var renewals = 0

        override val session =
            MutableStateFlow<Session?>(
                Session(
                    user = User(id = ANA_AS_ADMIN.userId, name = "Ana Souza", email = "ana@altarpm.com.br"),
                    tenant =
                        TenantSummary(
                            id = "0190f3c4-0000-7000-8000-000000000001",
                            name = "Alta RPM",
                            slug = "alta-rpm",
                        ),
                    role = Role.Admin,
                    permissions = GrantedPermissions.None,
                    memberships = emptyList(),
                ),
            )

        override suspend fun renew() {
            renewals++
        }
    }

    private companion object {
        val ANA_AS_ADMIN =
            Member(
                userId = "0190f3c4-0000-7000-8000-0000000000aa",
                name = "Ana Souza",
                email = "ana@altarpm.com.br",
                role = Role.Admin,
            )
        val BRUNO =
            Member(
                userId = "0190f3c4-0000-7000-8000-0000000000bb",
                name = "Bruno Lima",
                email = "bruno@altarpm.com.br",
                role = Role.Staff,
            )
    }
}
