package br.com.encomendo.feature.members

import br.com.encomendo.core.domain.members.ChangeMemberRole
import br.com.encomendo.core.domain.members.Invitation
import br.com.encomendo.core.domain.members.InviteMember
import br.com.encomendo.core.domain.members.ListInvitations
import br.com.encomendo.core.domain.members.ListMembers
import br.com.encomendo.core.domain.members.Member
import br.com.encomendo.core.domain.members.RemoveMember
import br.com.encomendo.core.domain.members.RevokeInvitation
import br.com.encomendo.core.domain.session.ObserveSession
import br.com.encomendo.core.domain.session.Role
import br.com.encomendo.core.testing.FakeInvitationsRepository
import br.com.encomendo.core.testing.FakeMembersRepository
import br.com.encomendo.core.testing.FakeSessionRepository
import br.com.encomendo.core.testing.testSession
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runCurrent
import kotlin.time.Instant

// The Equipe ViewModel tests' fakes and people.

/** The screen's ViewModel over the fakes, signed in as Ana, an admin. */
@OptIn(ExperimentalCoroutinesApi::class) // runCurrent
internal class Team(
    val scope: TestScope,
    val viewModel: MembersViewModel,
    val members: FakeMembersRepository,
    val invitations: FakeInvitationsRepository,
    val sessions: FakeSessionRepository,
) {
    /** The state once everything queued has run. */
    fun state(): MembersUiState {
        scope.runCurrent()
        return viewModel.uiState.value
    }
}

internal fun TestScope.team(
    invitationsSetUp: FakeInvitationsRepository.() -> Unit = {},
    configure: FakeMembersRepository.() -> Unit = {},
): Team {
    val members = FakeMembersRepository(listOf(CARLA, ANA, BRUNO)).apply(configure)
    val invitations = FakeInvitationsRepository(listOf(DIEGO)).apply(invitationsSetUp)
    val sessions = FakeSessionRepository(testSession(userId = ANA.userId, permissions = setOf("members:manage")))
    val useCases =
        MembersUseCases(
            listMembers = ListMembers(members, sessions),
            changeMemberRole = ChangeMemberRole(members, sessions),
            removeMember = RemoveMember(members, sessions),
            listInvitations = ListInvitations(invitations, sessions),
            inviteMember = InviteMember(invitations, sessions),
            revokeInvitation = RevokeInvitation(invitations, sessions),
        )
    val viewModel = MembersViewModel(observeSession = ObserveSession(sessions), useCases = useCases)
    // As the screen does: the state is shared while someone watches it.
    backgroundScope.launch { viewModel.uiState.collect {} }
    return Team(this, viewModel, members, invitations, sessions)
}

internal val CARLA =
    Member(
        userId = "0190f3c4-0000-7000-8000-0000000000cc",
        name = "Carla Dias",
        email = "carla@altarpm.com.br",
        role = Role.Owner,
    )

/** The signed-in member (testSession's default user id). */
internal val ANA =
    Member(
        userId = "0190f3c4-0000-7000-8000-0000000000aa",
        name = "Ana Souza",
        email = "ana@altarpm.com.br",
        role = Role.Admin,
    )

internal val BRUNO =
    Member(
        userId = "0190f3c4-0000-7000-8000-0000000000bb",
        name = "Bruno Lima",
        email = "bruno@altarpm.com.br",
        role = Role.Staff,
    )

/** A pending invitation, there when the screen opens. */
internal val DIEGO =
    Invitation(
        id = "0190f3c4-0000-7000-8000-0000000000e1",
        email = "diego@altarpm.com.br",
        role = Role.Viewer,
        expiresAt = Instant.parse("2026-11-30T12:00:00Z"),
    )
