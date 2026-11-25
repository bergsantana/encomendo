package br.com.encomendo.core.data.failure

import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.network.ApiError
import br.com.encomendo.core.network.ApiErrorCode
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertIs
import kotlin.test.assertSame

class ApiFailuresTest {
    @Test
    fun cancellationPassesThrough() =
        runTest {
            // A screen that leaves must stop its call, never see it as a failure (mobile/CLAUDE.md, Kotlin style).
            val cancelled = CancellationException("screen closed")

            val thrown = assertFailsWith<CancellationException> { asFailures { throw cancelled } }

            assertSame(cancelled, thrown)
        }

    @Test
    fun aRateLimitWithoutAProblemBodyIsStillARateLimit() {
        // A proxy can answer 429 with no problem body; the user still has to wait, not "try again".
        val failure = ApiError(status = 429, code = ApiErrorCode.Unknown).toFailure()

        assertIs<Failure.RateLimited>(failure)
    }

    @Test
    fun aRefusalIsForbidden() {
        assertIs<Failure.Forbidden>(ApiError(status = 403, code = ApiErrorCode.Forbidden).toFailure())
    }

    @Test
    fun aTakenAddressOrEmailNamesItsField() {
        // The screen puts its own words next to the field (web: features/auth/store.ts, fieldByCode).
        assertEquals("slug", assertIs<Failure.AlreadyTaken>(ApiError(409, ApiErrorCode.SlugTaken).toFailure()).field)
        assertEquals("email", assertIs<Failure.AlreadyTaken>(ApiError(409, ApiErrorCode.EmailTaken).toFailure()).field)
    }

    @Test
    fun codesTheAppDoesNotHandleAreUnexpected() {
        val failure = ApiError(status = 409, code = ApiErrorCode("plan_limit_reached")).toFailure()

        assertIs<Failure.Unexpected>(failure)
        assertEquals("plan_limit_reached", failure.code)
    }
}
