package br.com.encomendo.feature.members

import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.domain.failure.InputIssue
import br.com.encomendo.core.domain.members.Member
import br.com.encomendo.core.domain.paging.Page
import br.com.encomendo.core.domain.session.Role
import br.com.encomendo.core.ui.failure.FailureMessage
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.resetMain
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
class InvitationsViewModelTest {
    private val main = StandardTestDispatcher()

    @BeforeTest
    fun useTestMain() = Dispatchers.setMain(main)

    @AfterTest
    fun restoreMain() = Dispatchers.resetMain()

    @Test
    fun readsThePendingInvitations() =
        runTest(main) {
            assertEquals(listOf(DIEGO), team().state().invitations)
        }

    @Test
    fun anEmailChangedWhileItsInvitationGoesOutStays() =
        runTest(main) {
            // A typo fixed meanwhile is the next invitation, not one to lose.
            val gate = CompletableDeferred<Unit>()
            val team =
                team(
                    invitationsSetUp = {
                        val save = inviteAnswer
                        inviteAnswer = { email, role ->
                            gate.await()
                            save(email, role)
                        }
                    },
                )
            team.state()
            team.viewModel.onInviteEmailChange("erica@altarpm.com.br")
            team.viewModel.onSendInvitation()
            team.state()

            team.viewModel.onInviteEmailChange("fabio@altarpm.com.br")
            gate.complete(Unit)

            assertEquals("fabio@altarpm.com.br", team.state().invite.email)
        }

    @Test
    fun theInviteFormWorksWhileARowIsSaved() =
        runTest(main) {
            // One change to a row at a time; the form is not a row, as on the web.
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

            team.viewModel.onInviteEmailChange("erica@altarpm.com.br")
            team.viewModel.onSendInvitation()
            team.state()

            assertEquals(listOf("erica@altarpm.com.br" to Role.Staff), team.invitations.invites)
            gate.complete(Unit)
            team.state()
        }

    @Test
    fun aFailedLoadOfMoreInvitationsSaysSoAtTheTopAndKeepsTheRows() =
        runTest(main) {
            val team =
                team(
                    invitationsSetUp = {
                        pageAnswer = { cursor ->
                            if (cursor != null) throw Failure.Offline()
                            Page(listOf(DIEGO), nextCursor = "i2")
                        }
                    },
                )
            team.state()

            team.viewModel.onLoadMoreInvitations()

            val state = team.state()
            assertEquals(TeamMessage.General(FailureMessage.Offline), state.error)
            assertEquals(listOf(DIEGO), state.invitations)
            assertTrue(state.invitationsHaveMore, "Carregar mais convites stays, to try again")
            assertFalse(state.loadingMoreInvitations)
        }

    @Test
    fun eachRefusalAboutTheEmailAsksForTheKeyboardOnce() =
        runTest(main) {
            // As the web's focusEmail(): each tap that leaves the email marked takes the keyboard back to it.
            var answer: Failure = Failure.AlreadyMember()
            val team = team(invitationsSetUp = { inviteAnswer = { _, _ -> throw answer } })
            assertEquals(0, team.state().invite.emailFocusRequests)

            team.viewModel.onSendInvitation()
            team.viewModel.onSendInvitation()
            assertEquals(2, team.state().invite.emailFocusRequests, "twice blank, twice back to the email")

            team.viewModel.onInviteEmailChange("bruno@altarpm.com.br")
            team.viewModel.onSendInvitation()
            assertEquals(3, team.state().invite.emailFocusRequests, "on the team already")

            answer = Failure.Offline()
            team.viewModel.onSendInvitation()
            assertEquals(3, team.state().invite.emailFocusRequests, "offline is not about the email")
        }

    @Test
    fun anEmptyEmailIsMarkedAndNothingIsSent() =
        runTest(main) {
            // Marked on tap, never by a disabled button, as on every staff form.
            val team = team()
            team.state()

            team.viewModel.onInviteEmailChange("   ")
            team.viewModel.onSendInvitation()

            val form = team.state().invite
            assertEquals(EmailIssue.Missing, form.emailIssue, "Informe o e-mail.")
            assertEquals(TeamMessage.CheckFields, form.error, "Confira os campos destacados.")
            assertTrue(team.invitations.invites.isEmpty())
        }

    @Test
    fun anInvitationGoesOutListedFirstAndTheFormIsReadyForTheNext() =
        runTest(main) {
            val team = team()
            team.state()

            team.viewModel.onInviteEmailChange(" erica@altarpm.com.br ")
            team.viewModel.onInviteRoleChange(Role.Admin)
            team.viewModel.onSendInvitation()

            val state = team.state()
            assertEquals(listOf("erica@altarpm.com.br" to Role.Admin), team.invitations.invites)
            val sent = state.invitations.first()
            assertEquals("erica@altarpm.com.br", sent.email)
            assertEquals(listOf(sent, DIEGO), state.invitations)
            assertEquals(TeamNotice.InvitationSent(sent), state.notice, "Convite enviado para erica@altarpm.com.br.")
            assertEquals("", state.invite.email)
            assertEquals(Role.Admin, state.invite.role, "the role stays for the next invitation, as on the web")
            assertFalse(state.invite.sending)
        }

    @Test
    fun anInvitationIsForAFuncionarioUnlessAnotherRoleIsChosen() =
        runTest(main) {
            val team = team()

            assertEquals(Role.Staff, team.state().invite.role)
            team.viewModel.onInviteRoleChange(Role.Owner)
            assertEquals(Role.Staff, team.state().invite.role, "a store has one owner")
        }

    @Test
    fun someoneOnTheTeamIsMarkedNextToTheEmail() =
        runTest(main) {
            val team = team(invitationsSetUp = { inviteAnswer = { _, _ -> throw Failure.AlreadyMember() } })
            team.state()

            team.viewModel.onInviteEmailChange("bruno@altarpm.com.br")
            team.viewModel.onSendInvitation()

            val form = team.state().invite
            assertEquals(EmailIssue.AlreadyOnTeam, form.emailIssue, "Esta pessoa já faz parte da equipe.")
            assertEquals(TeamMessage.CheckFields, form.error)
            assertEquals("bruno@altarpm.com.br", form.email, "kept, to fix")

            team.viewModel.onInviteEmailChange("bruna@altarpm.com.br")
            assertNull(team.state().invite.emailIssue)
            assertNull(team.state().invite.error)
        }

    @Test
    fun theServersWordsForTheEmailShowUnderIt() =
        runTest(main) {
            val refused = Failure.InvalidInput(listOf(InputIssue("email", "E-mail inválido.")))
            val team = team(invitationsSetUp = { inviteAnswer = { _, _ -> throw refused } })
            team.state()

            team.viewModel.onInviteEmailChange("erica@")
            team.viewModel.onSendInvitation()

            val form = team.state().invite
            assertEquals(EmailIssue.Server("E-mail inválido."), form.emailIssue)
            assertEquals(TeamMessage.CheckFields, form.error)
        }

    @Test
    fun aRateLimitsWaitStaysWhileTyping() =
        runTest(main) {
            val team =
                team(invitationsSetUp = {
                    inviteAnswer =
                        { _, _ -> throw Failure.RateLimited(retryAfterSeconds = 30) }
                })
            team.state()
            team.viewModel.onInviteEmailChange("erica@altarpm.com.br")
            team.viewModel.onSendInvitation()

            team.viewModel.onInviteEmailChange("erika@altarpm.com.br")

            assertEquals(
                TeamMessage.General(FailureMessage.RateLimited(30)),
                team.state().invite.error,
                "typing doesn't shorten it",
            )
        }

    @Test
    fun anInvitationSavedWithoutItsEmailSaysSoAndIsListed() =
        runTest(main) {
            val team =
                team(
                    invitationsSetUp = {
                        val save = inviteAnswer
                        inviteAnswer = { email, role ->
                            save(email, role)
                            throw Failure.EmailNotSent()
                        }
                    },
                )
            team.state()

            team.viewModel.onInviteEmailChange("erica@altarpm.com.br")
            team.viewModel.onSendInvitation()

            val state = team.state()
            assertEquals(TeamMessage.NotSent, state.invite.error, "O convite foi salvo, mas o e-mail não saiu.")
            assertEquals(listOf("erica@altarpm.com.br", DIEGO.email), state.invitations.map { it.email })
        }

    @Test
    fun resendingGivesTheInvitationANewLinkInTheSameRow() =
        runTest(main) {
            val team = team()
            team.state()

            team.viewModel.onResend(DIEGO)

            val state = team.state()
            assertEquals(listOf(DIEGO.email to DIEGO.role), team.invitations.invites)
            val renewed = state.invitations.single()
            assertEquals(DIEGO.key, renewed.key)
            assertEquals(
                TeamNotice.InvitationResent(renewed),
                state.notice,
                "Convite reenviado para diego@altarpm.com.br.",
            )
        }

    @Test
    fun cancellingAnInvitationTakesItsRowAway() =
        runTest(main) {
            val team = team()
            team.state()

            team.viewModel.onRevoke(DIEGO)

            val state = team.state()
            assertEquals(listOf(DIEGO.id), team.invitations.revocations)
            assertTrue(state.invitations.isEmpty())
            assertEquals(
                TeamNotice.InvitationRevoked(DIEGO),
                state.notice,
                "Convite para diego@altarpm.com.br cancelado.",
            )
        }

    @Test
    fun anInvitationNoLongerPendingShowsAtTheTopAndTheListsAreReadAgain() =
        runTest(main) {
            val team = team(invitationsSetUp = { revokeAnswer = { throw Failure.NotFound() } })
            team.state()
            team.invitations.pending = emptyList()

            team.viewModel.onRevoke(DIEGO)

            val state = team.state()
            assertEquals(TeamMessage.NotPending, state.error, "Este convite não está mais pendente.")
            assertNull(state.rowError)
            assertTrue(state.invitations.isEmpty())
        }

    @Test
    fun someoneWhoJoinedMeanwhileShowsAtTheTopAndTheListsAreReadAgain() =
        runTest(main) {
            val diego =
                Member(
                    userId = "0190f3c4-0000-7000-8000-0000000000dd",
                    name = "Diego Alves",
                    email = DIEGO.email,
                    role = Role.Viewer,
                )
            val team = team(invitationsSetUp = { inviteAnswer = { _, _ -> throw Failure.AlreadyMember() } })
            team.state()
            team.members.team = listOf(CARLA, ANA, BRUNO, diego)
            team.invitations.pending = emptyList()

            team.viewModel.onResend(DIEGO)

            val state = team.state()
            assertEquals(TeamMessage.AlreadyOnTeam, state.error, "Esta pessoa já faz parte da equipe.")
            assertEquals(listOf(CARLA, ANA, BRUNO, diego), state.members)
            assertTrue(state.invitations.isEmpty())
        }

    @Test
    fun aResendWhoseEmailDidNotGoOutSaysSoInItsRow() =
        runTest(main) {
            // Its link and expiry are new all the same, so the invitations are read again.
            val team =
                team(
                    invitationsSetUp = {
                        val save = inviteAnswer
                        inviteAnswer = { email, role ->
                            save(email, role)
                            throw Failure.EmailNotSent()
                        }
                    },
                )
            team.state()

            team.viewModel.onResend(DIEGO)

            val state = team.state()
            assertEquals(RowError(RowKey.OfInvitation(DIEGO.key), TeamMessage.NotSent), state.rowError)
            assertEquals(listOf(DIEGO.key), state.invitations.map { it.key })
            assertFalse(state.invitations.single().id == DIEGO.id, "the re-read has the new invitation")
        }

    @Test
    fun invitationsPastOnePageLoadMore() =
        runTest(main) {
            val erica = DIEGO.copy(id = "0190f3c4-0000-7000-8000-0000000000e2", email = "erica@altarpm.com.br")
            val team =
                team(
                    invitationsSetUp = {
                        pageAnswer = { cursor ->
                            if (cursor ==
                                null
                            ) {
                                Page(listOf(DIEGO), nextCursor = "i2")
                            } else {
                                Page(listOf(erica), nextCursor = null)
                            }
                        }
                    },
                )
            assertTrue(team.state().invitationsHaveMore, "Carregar mais convites")

            team.viewModel.onLoadMoreInvitations()

            val state = team.state()
            assertEquals(listOf(DIEGO, erica), state.invitations)
            assertFalse(state.invitationsHaveMore)
            assertFalse(state.loadingMoreInvitations)
        }

    @Test
    fun oneChangeAtATimeAcrossBothLists() =
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

            team.viewModel.onResend(DIEGO)
            team.viewModel.onRevoke(DIEGO)
            gate.complete(Unit)
            team.state()

            assertTrue(team.invitations.invites.isEmpty())
            assertTrue(team.invitations.revocations.isEmpty())
        }
}
