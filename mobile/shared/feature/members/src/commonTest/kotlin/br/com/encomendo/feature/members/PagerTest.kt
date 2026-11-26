package br.com.encomendo.feature.members

import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.domain.paging.Page
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.UnconfinedTestDispatcher
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

@OptIn(ExperimentalCoroutinesApi::class) // runCurrent, UnconfinedTestDispatcher
class PagerTest {
    @Test
    fun aReadReplacedByAnotherIsDroppedAndOnlyTheLatestEnds() =
        runTest {
            val first = CompletableDeferred<Unit>()
            var calls = 0
            val pager =
                Pager(this) { _ ->
                    if (++calls == 1) first.await()
                    Page(listOf("read $calls"), nextCursor = null)
                }
            val log = mutableListOf<String>()
            val reader = PageReader<String>(onPage = { log += it.items }, onFailure = {}, onEnd = { log += "end" })

            pager.readFirst(reader)
            runCurrent()
            pager.readFirst(reader)
            runCurrent()
            first.complete(Unit)
            runCurrent()

            assertEquals(listOf("read 2", "end"), log)
        }

    @Test
    fun thereIsNoNextPageAfterTheLastOrWhileAReadIsUnderWay() =
        runTest {
            val gate = CompletableDeferred<Unit>()
            val pager =
                Pager(this) { cursor ->
                    if (cursor == "p2") gate.await()
                    Page(listOf(cursor.orEmpty()), nextCursor = if (cursor == null) "p2" else null)
                }
            val reader = PageReader<String>(onPage = {}, onFailure = {}, onEnd = {})

            assertFalse(pager.readNext(reader), "no page read yet, so no cursor")
            pager.readFirst(reader)
            runCurrent()
            assertTrue(pager.readNext(reader))
            assertFalse(pager.readNext(reader), "the next page is still being read")
            gate.complete(Unit)
            runCurrent()
            assertFalse(pager.readNext(reader), "that was the last page")
        }

    @Test
    fun whatAReadMarksAsItStartsIsThereBeforeItCanEnd() =
        runTest(UnconfinedTestDispatcher()) {
            // On an immediate dispatcher a read that fails at once ends inside the launch.
            val pager = Pager<String>(this) { throw Failure.Offline() }
            val log = mutableListOf<String>()

            pager.readFirst(
                PageReader(
                    onStart = { log += "start" },
                    onPage = {},
                    onFailure = { log += "failure" },
                    onEnd = { log += "end" },
                ),
            )

            assertEquals(listOf("start", "failure", "end"), log)
        }
}
