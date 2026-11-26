package br.com.encomendo.feature.members

import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.domain.members.ChangeMemberRole
import br.com.encomendo.core.domain.members.ListMembers
import br.com.encomendo.core.domain.members.Member
import br.com.encomendo.core.domain.members.MemberPage
import br.com.encomendo.core.domain.members.RemoveMember
import br.com.encomendo.core.domain.session.ObserveSession
import br.com.encomendo.core.domain.session.Role
import br.com.encomendo.core.testing.FakeMembersRepository
import br.com.encomendo.core.testing.FakeSessionRepository
import br.com.encomendo.core.testing.testSession
import br.com.encomendo.core.ui.failure.FailureMessage
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import kotlin.test.AfterTest
import kotlin.test.BeforeTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

@OptIn(ExperimentalCoroutinesApi::class) // setMain / resetMain / runCurrent
class MembersViewModelTest {
    private val main = StandardTestDispatcher()

    @BeforeTest
    fun useTestMain() = Dispatchers.setMain(main)

    @AfterTest
    fun restoreMain() = Dispatchers.resetMain()

    @Test
    fun readsTheTeamAndKnowsWhoIsSignedIn() =
        runTest(main) {
            val team = team()

            val state = team.state()

            assertEquals(listOf(CARLA, ANA, BRUNO), state.members)
            assertTrue(state.isMe(ANA), "Ana's row says (você)")
            assertFalse(state.isMe(BRUNO))
            assertFalse(state.loading)
            assertFalse(state.hasMore)
        }

    @Test
    fun saysItIsLoadingUntilTheTeamArrives() =
        runTest(main) {
            val gate = CompletableDeferred<Unit>()
            val team = team { pageAnswer = { gate.await().let { MemberPage(team, null) } } }

            assertTrue(team.state().loading, "Carregando a equipe…")
            gate.complete(Unit)
            assertFalse(team.state().loading)
        }

    @Test
    fun aTeamPastOnePageLoadsMore() =
        runTest(main) {
            val team =
                team {
                    pageAnswer = { cursor ->
                        if (cursor ==
                            null
                        ) {
                            MemberPage(listOf(CARLA, ANA), nextCursor = "c2")
                        } else {
                            MemberPage(listOf(BRUNO), null)
                        }
                    }
                }
            assertTrue(team.state().hasMore, "Carregar mais")

            team.viewModel.onLoadMore()

            val state = team.state()
            assertEquals(listOf(CARLA, ANA, BRUNO), state.members)
            assertFalse(state.hasMore)
            assertEquals(listOf(null, "c2"), team.members.pagesRead)
        }

    @Test
    fun aTeamThatCannotBeReadOffersToTryAgain() =
        runTest(main) {
            var offline = true
            val team = team { pageAnswer = { if (offline) throw Failure.Offline() else MemberPage(team, null) } }

            val failed = team.state()
            assertEquals(TeamMessage.General(FailureMessage.Offline), failed.error)
            assertTrue(failed.canRetry, "Tentar de novo")

            offline = false
            team.viewModel.onRetry()

            val state = team.state()
            assertNull(state.error)
            assertFalse(state.canRetry)
            assertEquals(listOf(CARLA, ANA, BRUNO), state.members)
        }

    @Test
    fun aLostPermissionSaysSoAndCatchesTheSessionUp() =
        runTest(main) {
            // The screen then leaves for Hoje once the session catches up (design document, "Equipe").
            val team = team { pageAnswer = { throw Failure.Forbidden() } }

            assertEquals(TeamMessage.LostPermission, team.state().error)
            assertEquals(1, team.sessions.renewals)
        }

    @Test
    fun pickingARoleOnlyDraftsIt() =
        runTest(main) {
            // Arrow keys on the choice never save a role step by step (WCAG 3.2.2), as on the web.
            val team = team()
            team.state()

            team.viewModel.onPickRole(BRUNO, Role.Viewer)

            val state = team.state()
            assertEquals(Role.Viewer, state.draftFor(BRUNO))
            assertTrue(state.hasDraft(BRUNO), "Alterar papel appears")
            assertTrue(team.members.changes.isEmpty())

            team.viewModel.onPickRole(BRUNO, Role.Staff)
            assertFalse(team.state().hasDraft(BRUNO), "back to the role Bruno has")
        }

    @Test
    fun savingADraftChangesTheRoleAndSaysSo() =
        runTest(main) {
            val team = team()
            team.state()

            team.viewModel.onPickRole(BRUNO, Role.Viewer)
            team.viewModel.onSaveRole(BRUNO)

            val state = team.state()
            val changed = BRUNO.copy(role = Role.Viewer)
            assertEquals(listOf(BRUNO.userId to Role.Viewer), team.members.changes)
            assertEquals(listOf(CARLA, ANA, changed), state.members)
            assertEquals(TeamNotice.RoleChanged(changed), state.notice, "Bruno Lima agora é Visualizador.")
            assertFalse(state.hasDraft(BRUNO))
            assertEquals(0, team.sessions.renewals, "someone else's role leaves the session as it is")
        }

    @Test
    fun onesOwnRoleAsksFirstAndCancelPutsItBack() =
        runTest(main) {
            val team = team()
            team.state()

            team.viewModel.onPickRole(ANA, Role.Staff)
            team.viewModel.onSaveRole(ANA)

            assertEquals(Question(ANA.userId, QuestionKind.OwnRole), team.state().question)
            assertTrue(team.members.changes.isEmpty(), "Você deixará de gerenciar a equipe.")

            team.viewModel.onCancel()

            val state = team.state()
            assertNull(state.question)
            assertEquals(Role.Admin, state.draftFor(ANA), "Cancelar restores the role Ana has")
        }

    @Test
    fun confirmingOnesOwnRoleChangesItAndCatchesTheSessionUp() =
        runTest(main) {
            val team = team()
            team.state()

            team.viewModel.onPickRole(ANA, Role.Staff)
            team.viewModel.onSaveRole(ANA)
            team.viewModel.onConfirm()

            assertNull(team.state().question)
            assertEquals(listOf(ANA.userId to Role.Staff), team.members.changes)
            assertEquals(1, team.sessions.renewals, "Equipe goes from the menu at once")
        }

    @Test
    fun removingAsksFirstThenRemoves() =
        runTest(main) {
            val team = team()
            team.state()

            team.viewModel.onAskRemove(BRUNO)

            assertEquals(Question(BRUNO.userId, QuestionKind.Removal), team.state().question)
            assertTrue(team.members.removals.isEmpty(), "Remover Bruno Lima da equipe?")

            team.viewModel.onConfirm()

            val state = team.state()
            assertEquals(listOf(BRUNO.userId), team.members.removals)
            assertEquals(listOf(CARLA, ANA), state.members)
            assertEquals(TeamNotice.Removed(BRUNO), state.notice, "Bruno Lima saiu da equipe.")
            assertNull(state.question)
        }

    @Test
    fun cancellingARemovalKeepsTheRow() =
        runTest(main) {
            val team = team()
            team.state()

            team.viewModel.onAskRemove(BRUNO)
            team.viewModel.onCancel()

            assertNull(team.state().question)
            assertTrue(team.members.removals.isEmpty())
        }

    @Test
    fun theOwnersRowHasNothingToChange() =
        runTest(main) {
            val team = team()
            team.state()

            team.viewModel.onPickRole(CARLA, Role.Admin)
            team.viewModel.onAskRemove(CARLA)

            val state = team.state()
            assertFalse(state.hasDraft(CARLA))
            assertNull(state.question)
        }

    @Test
    fun aFailedChangeShowsInItsRow() =
        runTest(main) {
            val team = team { changeAnswer = { _, _ -> throw Failure.Forbidden() } }
            team.state()

            team.viewModel.onPickRole(BRUNO, Role.Viewer)
            team.viewModel.onSaveRole(BRUNO)

            val state = team.state()
            assertEquals(RowError(BRUNO.userId, TeamMessage.LostPermission), state.rowError)
            assertNull(state.error)
            assertNull(state.savingUserId)
            assertTrue(state.hasDraft(BRUNO), "the draft stays, to try again")
            assertEquals(1, team.sessions.renewals)
        }

    @Test
    fun aConnectionFailureShowsInTheRowInTheWordsOfEveryScreen() =
        runTest(main) {
            val team = team { removeAnswer = { throw Failure.Offline() } }
            team.state()

            team.viewModel.onAskRemove(BRUNO)
            team.viewModel.onConfirm()

            assertEquals(RowError(BRUNO.userId, TeamMessage.General(FailureMessage.Offline)), team.state().rowError)
        }

    @Test
    fun someoneGoneMeanwhileShowsAtTheTopAndTheTeamIsReadAgain() =
        runTest(main) {
            // That row goes when the list is read again, so the message cannot stay in it.
            val team = team()
            team.state()
            team.members.team = listOf(CARLA, ANA)
            team.members.changeAnswer = { _, _ -> throw Failure.NotFound() }

            team.viewModel.onPickRole(BRUNO, Role.Viewer)
            team.viewModel.onSaveRole(BRUNO)

            val state = team.state()
            assertEquals(TeamMessage.NotOnTeam, state.error, "Esta pessoa não faz mais parte da equipe.")
            assertNull(state.rowError)
            assertEquals(listOf(CARLA, ANA), state.members)
            assertFalse(state.hasDraft(BRUNO))
        }

    @Test
    fun aRoleChangedElsewhereReplacesTheDraftWhenTheTeamIsReadAgain() =
        runTest(main) {
            // As the web's row does when a saved or re-read role arrives.
            val team = team()
            team.state()
            team.viewModel.onPickRole(ANA, Role.Viewer)
            team.members.team = listOf(CARLA, ANA, BRUNO.copy(role = Role.Admin))
            team.members.changeAnswer = { _, _ -> throw Failure.NotFound() }
            team.viewModel.onPickRole(BRUNO, Role.Viewer)

            team.viewModel.onSaveRole(BRUNO)

            val state = team.state()
            assertFalse(state.hasDraft(BRUNO), "Bruno's role changed meanwhile")
            assertEquals(Role.Viewer, state.draftFor(ANA), "Ana's role did not, so her draft stays")
        }

    @Test
    fun oneChangeAtATime() =
        runTest(main) {
            val gate = CompletableDeferred<Unit>()
            val team =
                team {
                    val save = changeAnswer
                    changeAnswer = { userId, role ->
                        gate.await()
                        save(userId, role)
                    }
                }
            team.state()

            team.viewModel.onPickRole(BRUNO, Role.Viewer)
            team.viewModel.onSaveRole(BRUNO)
            assertEquals(BRUNO.userId, team.state().savingUserId)
            team.viewModel.onAskRemove(ANA)
            team.viewModel.onSaveRole(BRUNO)

            assertNull(team.state().question, "no question while a change is saved")
            gate.complete(Unit)
            assertNull(team.state().savingUserId)
            assertEquals(1, team.members.changes.size)
        }

    @Test
    fun aNewChangeClearsTheLastMessages() =
        runTest(main) {
            val team = team()
            team.state()
            team.viewModel.onAskRemove(BRUNO)
            team.viewModel.onConfirm()
            assertEquals(TeamNotice.Removed(BRUNO), team.state().notice)

            team.viewModel.onPickRole(ANA, Role.Viewer)
            team.viewModel.onSaveRole(ANA)
            team.viewModel.onConfirm()

            assertEquals(TeamNotice.RoleChanged(ANA.copy(role = Role.Viewer)), team.state().notice)
        }

    @Test
    fun aFailedLoadMoreSaysSoAtTheTopAndKeepsTheRows() =
        runTest(main) {
            val team =
                team {
                    pageAnswer = { cursor ->
                        if (cursor != null) throw Failure.Offline()
                        MemberPage(listOf(CARLA, ANA), nextCursor = "c2")
                    }
                }
            team.state()

            team.viewModel.onLoadMore()

            val state = team.state()
            assertEquals(TeamMessage.General(FailureMessage.Offline), state.error)
            assertEquals(listOf(CARLA, ANA), state.members)
            assertTrue(state.hasMore, "Carregar mais stays, to try again")
            assertFalse(state.loadingMore)
            assertFalse(state.canRetry, "the rows show, so there is nothing to read from the start")
        }

    @Test
    fun aPageAskedForBeforeTheTeamIsReadAgainIsDropped() =
        runTest(main) {
            // It would follow a list no longer on screen: rows twice, or out of order.
            val secondPage = CompletableDeferred<Unit>()
            val team =
                team {
                    pageAnswer = { cursor ->
                        if (cursor == null) {
                            MemberPage(team.take(2), nextCursor = "c2")
                        } else {
                            secondPage.await()
                            MemberPage(team.drop(2), nextCursor = null)
                        }
                    }
                    changeAnswer = { _, _ -> throw Failure.NotFound() }
                }
            team.state()
            team.viewModel.onLoadMore()
            team.members.team = listOf(CARLA, BRUNO, ANA)

            // Ana's change finds the team changed, so the team is read again while the second page waits.
            team.viewModel.onPickRole(ANA, Role.Viewer)
            team.viewModel.onSaveRole(ANA)
            team.viewModel.onConfirm()
            team.state()
            secondPage.complete(Unit)

            val state = team.state()
            assertEquals(listOf(CARLA, BRUNO), state.members, "the first page as read again, nothing after it")
            assertTrue(state.hasMore)
            assertFalse(state.loadingMore)
        }

    @Test
    fun aReReadReplacedByAnotherStillLoadsUntilTheLastEnds() =
        runTest(main) {
            // Two changes find the team changed, one after the other: the first re-read is cancelled,
            // and its end must not say the second one is done.
            val firstReRead = CompletableDeferred<Unit>()
            val lastReRead = CompletableDeferred<Unit>()
            var reads = 0
            val team =
                team {
                    pageAnswer = {
                        reads++
                        if (reads == 2) firstReRead.await()
                        if (reads == 3) lastReRead.await()
                        MemberPage(team, nextCursor = null)
                    }
                    changeAnswer = { _, _ -> throw Failure.NotFound() }
                }
            team.state()
            team.viewModel.onPickRole(BRUNO, Role.Viewer)
            team.viewModel.onSaveRole(BRUNO)
            team.state()

            team.viewModel.onSaveRole(BRUNO)

            assertTrue(team.state().loading, "the second re-read is still under way")
            lastReRead.complete(Unit)
            assertFalse(team.state().loading)
            firstReRead.complete(Unit)
        }

    @Test
    fun confirmingWhileAnotherChangeSavesDoesNothing() =
        runTest(main) {
            val gate = CompletableDeferred<Unit>()
            val team =
                team {
                    val save = changeAnswer
                    changeAnswer = { userId, role ->
                        gate.await()
                        save(userId, role)
                    }
                }
            team.state()
            team.viewModel.onPickRole(BRUNO, Role.Viewer)
            team.viewModel.onAskRemove(BRUNO)
            team.viewModel.onSaveRole(BRUNO)
            assertEquals(BRUNO.userId, team.state().savingUserId)

            team.viewModel.onConfirm()
            gate.complete(Unit)
            team.state()

            assertTrue(team.members.removals.isEmpty(), "one change at a time")
            assertEquals(listOf(BRUNO.userId to Role.Viewer), team.members.changes)
        }

    @Test
    fun removingOneselfTakesTheRowAwayAndCatchesTheSessionUp() =
        runTest(main) {
            // The renewed session has no such store, and the app leaves it (ADR 0010).
            val team = team()
            team.state()

            team.viewModel.onAskRemove(ANA)
            assertEquals(Question(ANA.userId, QuestionKind.Removal), team.state().question)
            team.viewModel.onConfirm()

            val state = team.state()
            assertEquals(listOf(CARLA, BRUNO), state.members)
            assertEquals(TeamNotice.Removed(ANA), state.notice)
            assertEquals(1, team.sessions.renewals)
        }

    @Test
    fun aFailedReReadKeepsTheMessageAtTheTop() =
        runTest(main) {
            val team = team()
            team.state()
            team.members.changeAnswer = { _, _ -> throw Failure.NotFound() }
            team.members.pageAnswer = { throw Failure.Offline() }

            team.viewModel.onPickRole(BRUNO, Role.Viewer)
            team.viewModel.onSaveRole(BRUNO)

            val state = team.state()
            assertEquals(TeamMessage.NotOnTeam, state.error, "not hidden by the failed re-read")
            assertEquals(listOf(CARLA, ANA, BRUNO), state.members)
            assertFalse(state.loading)
        }

    @Test
    fun rolesWaitWhileAChangeIsSavedOrTheRowAsks() =
        runTest(main) {
            val gate = CompletableDeferred<Unit>()
            val team =
                team {
                    removeAnswer = {
                        gate.await()
                        team = team.filter { it.userId != BRUNO.userId }
                    }
                }
            team.state()

            team.viewModel.onPickRole(ANA, Role.Staff)
            team.viewModel.onSaveRole(ANA)
            team.viewModel.onPickRole(ANA, Role.Admin)
            assertEquals(Role.Staff, team.state().draftFor(ANA), "the question is about the role it showed")

            team.viewModel.onCancel()
            team.viewModel.onAskRemove(BRUNO)
            team.viewModel.onConfirm()
            team.viewModel.onPickRole(ANA, Role.Viewer)

            assertFalse(team.state().hasDraft(ANA), "no draft while a change is saved")
            gate.complete(Unit)
            team.state()
        }

    @Test
    fun aQuestionAboutOnesOwnRoleGoesWithItsDraft() =
        runTest(main) {
            val team = team()
            team.state()
            team.viewModel.onPickRole(ANA, Role.Staff)
            team.viewModel.onSaveRole(ANA)
            team.state()
            team.members.team = listOf(CARLA, ANA.copy(role = Role.Viewer), BRUNO)

            team.viewModel.onRetry()

            val state = team.state()
            assertFalse(state.hasDraft(ANA), "Ana's role changed elsewhere")
            assertNull(state.question, "nothing is left to confirm")
        }

    @Test
    fun theStateInALogCarriesNoPersonalData() =
        runTest(main) {
            val printed = team().state().toString()

            listOf("Carla", "Ana", "Bruno", "@altarpm").forEach { assertFalse(it in printed, printed) }
        }

    /** The screen's ViewModel over [FakeMembersRepository], signed in as Ana, an admin. */
    private class Team(
        val scope: TestScope,
        val viewModel: MembersViewModel,
        val members: FakeMembersRepository,
        val sessions: FakeSessionRepository,
    ) {
        /** The state once everything queued has run. */
        fun state(): MembersUiState {
            scope.runCurrent()
            return viewModel.uiState.value
        }
    }

    private fun TestScope.team(configure: FakeMembersRepository.() -> Unit = {}): Team {
        val members = FakeMembersRepository(listOf(CARLA, ANA, BRUNO)).apply(configure)
        val sessions = FakeSessionRepository(testSession(userId = ANA.userId, permissions = setOf("members:manage")))
        val viewModel =
            MembersViewModel(
                observeSession = ObserveSession(sessions),
                listMembers = ListMembers(members, sessions),
                changeMemberRole = ChangeMemberRole(members, sessions),
                removeMember = RemoveMember(members, sessions),
            )
        // As the screen does: the state is shared while someone watches it.
        backgroundScope.launch { viewModel.uiState.collect {} }
        return Team(this, viewModel, members, sessions)
    }

    private companion object {
        val CARLA =
            Member(
                userId = "0190f3c4-0000-7000-8000-0000000000cc",
                name = "Carla Dias",
                email = "carla@altarpm.com.br",
                role = Role.Owner,
            )

        /** The signed-in member (testSession's default user id). */
        val ANA =
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
