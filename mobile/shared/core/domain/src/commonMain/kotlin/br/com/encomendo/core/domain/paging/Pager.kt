package br.com.encomendo.core.domain.paging

import br.com.encomendo.core.domain.failure.Failure
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.launch

/**
 * Pager reads one list a page at a time in [scope]: from the start, or the next page. Reading from the start cancels
 * a read under way, whose page is then dropped, so a list never mixes the pages of two reads. Only the latest read
 * reports that it ended: a cancelled one may end after its replacement started. Used on the main thread only.
 */
class Pager<T>(
    private val scope: CoroutineScope,
    private val read: suspend (cursor: String?) -> Page<T>,
) {
    private var nextCursor: String? = null
    private var job: Job? = null
    private var reads = 0

    /** Reads the first page, in place of any read under way. */
    fun readFirst(reader: PageReader<T>) = start(cursor = null, reader)

    /** Reads the next page; does nothing and returns false on the last page or while a read is under way. */
    fun readNext(reader: PageReader<T>): Boolean {
        val cursor = nextCursor?.takeUnless { job?.isActive == true } ?: return false
        start(cursor, reader)
        return true
    }

    private fun start(
        cursor: String?,
        reader: PageReader<T>,
    ) {
        job?.cancel()
        val number = ++reads
        // Before the launch: on an immediate dispatcher the read may end inside it.
        reader.onStart()
        job =
            scope.launch {
                try {
                    val page = read(cursor)
                    nextCursor = page.nextCursor
                    reader.onPage(page)
                } catch (failure: Failure) {
                    reader.onFailure(failure)
                } finally {
                    if (number == reads) reader.onEnd()
                }
            }
    }
}

/** PageReader is what a [Pager]'s caller does as a read starts, with a page, with a failure, and once it ends. */
class PageReader<T>(
    /** Called as a read starts, before anything is read. */
    val onStart: () -> Unit = {},
    /** Called with the page read. */
    val onPage: (Page<T>) -> Unit,
    /** Called when the read failed. */
    val onFailure: (Failure) -> Unit,
    /** Called once when the latest read ends, whatever its result. */
    val onEnd: () -> Unit,
)
