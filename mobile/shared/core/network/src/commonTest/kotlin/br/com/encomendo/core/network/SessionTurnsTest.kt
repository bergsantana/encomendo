package br.com.encomendo.core.network

import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.async
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertFalse
import kotlin.test.assertTrue

@OptIn(ExperimentalCoroutinesApi::class) // runCurrent
class SessionTurnsTest {
    @Test
    fun oneTurnAtATime() =
        runTest {
            val turns = SessionTurns()
            val firstDone = CompletableDeferred<Unit>()
            launch { turns.take { firstDone.await() } }
            runCurrent()

            val second = async { turns.take { "second" } }
            runCurrent()
            assertFalse(second.isCompleted, "waits for the first turn")

            firstDone.complete(Unit)
            assertEquals("second", second.await())
        }

    @Test
    fun aTurnInsideATurnFailsAtOnceInsteadOfWaitingForever() =
        runTest {
            val turns = SessionTurns()

            turns.take {
                assertTrue(SessionTurns.inTurn())
                assertFailsWith<IllegalStateException> { turns.take { } }
            }
            assertFalse(SessionTurns.inTurn())
        }
}
