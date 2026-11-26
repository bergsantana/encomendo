package br.com.encomendo.core.data.session

import br.com.encomendo.core.domain.session.Session
import br.com.encomendo.core.testing.AltaRpm
import br.com.encomendo.core.testing.testSession
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

@OptIn(ExperimentalCoroutinesApi::class) // runCurrent
class InMemoryStoreSuspensionNoticeTest {
    private val ana = testSession(userId = "u-ana")
    private val bruno = testSession(userId = "u-bruno")

    @Test
    fun aRenewalOfTheSameUserKeepsTheNotice() =
        runTest(StandardTestDispatcher()) {
            val sessions = MutableStateFlow<Session?>(ana)
            val notice = InMemoryStoreSuspensionNotice()
            backgroundScope.launch { notice.follow(sessions) }
            runCurrent()

            notice.raise(AltaRpm.id)
            sessions.value = ana.copy(tenant = null) // the renewal: same user, no store now
            runCurrent()

            assertEquals(AltaRpm.id, notice.raisedFor.value)
        }

    @Test
    fun signingOutOrChangingUserDropsTheNotice() =
        runTest(StandardTestDispatcher()) {
            val sessions = MutableStateFlow<Session?>(ana)
            val notice = InMemoryStoreSuspensionNotice()
            backgroundScope.launch { notice.follow(sessions) }
            runCurrent()

            notice.raise(AltaRpm.id)
            sessions.value = null
            runCurrent()
            assertNull(notice.raisedFor.value)

            notice.raise(AltaRpm.id)
            sessions.value = bruno
            runCurrent()
            assertNull(notice.raisedFor.value)
        }
}
