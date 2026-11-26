package br.com.encomendo.feature.members

import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.domain.paging.Page
import br.com.encomendo.core.ui.failure.FailureMessage
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.UnconfinedTestDispatcher
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import kotlin.test.AfterTest
import kotlin.test.BeforeTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse

/**
 * Equipe on an immediate main dispatcher, as `viewModelScope` runs on the
 * device: an answer that comes back at once must leave the screen as a slow
 * one does.
 */
@OptIn(ExperimentalCoroutinesApi::class) // setMain / resetMain, UnconfinedTestDispatcher
class ImmediateAnswersTest {
    private val main = UnconfinedTestDispatcher()

    @BeforeTest
    fun useTestMain() = Dispatchers.setMain(main)

    @AfterTest
    fun restoreMain() = Dispatchers.resetMain()

    @Test
    fun aNextPageThatFailsAtOnceSaysSoAndCanBeAskedForAgain() =
        runTest(main) {
            val team =
                team(
                    invitationsSetUp = {
                        pageAnswer = { cursor ->
                            if (cursor != null) throw Failure.Offline()
                            Page(listOf(DIEGO), nextCursor = "i2")
                        }
                    },
                ) {
                    pageAnswer = { cursor ->
                        if (cursor != null) throw Failure.Offline()
                        Page(listOf(CARLA, ANA), nextCursor = "c2")
                    }
                }

            team.viewModel.onLoadMore()

            val afterTeam = team.state()
            assertEquals(TeamMessage.General(FailureMessage.Offline), afterTeam.error)
            assertFalse(afterTeam.loadingMore, "Carregar mais can be tapped again")

            team.viewModel.onLoadMoreInvitations()

            val afterInvitations = team.state()
            assertEquals(TeamMessage.General(FailureMessage.Offline), afterInvitations.error)
            assertFalse(afterInvitations.loadingMoreInvitations, "Carregar mais convites can be tapped again")
        }
}
