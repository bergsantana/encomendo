package br.com.encomendo.core.ui.failure

import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.domain.failure.InputIssue
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

class FailureMessageTest {
    @Test
    fun offlineAndRateLimitsHaveTheirOwnWords() {
        assertEquals(FailureMessage.Offline, Failure.Offline().toFailureMessage())
        assertEquals(FailureMessage.RateLimited(30), Failure.RateLimited(retryAfterSeconds = 30).toFailureMessage())
        assertEquals(FailureMessage.RateLimited(null), Failure.RateLimited(retryAfterSeconds = null).toFailureMessage())
    }

    @Test
    fun aRefusalShowsTheServersOwnWords() {
        assertEquals(
            FailureMessage.Refused("Conta bloqueada"),
            Failure.Unexpected(409, "conflict", "Conta bloqueada").toFailureMessage(),
        )
        assertEquals(
            FailureMessage.Refused("Sem permissão"),
            Failure.Forbidden(title = "Sem permissão").toFailureMessage(),
        )
    }

    @Test
    fun aRefusalWithoutWordsIsAGeneralError() {
        // Never an empty message.
        assertEquals(FailureMessage.Unexpected, Failure.Unexpected(409, "conflict", title = "  ").toFailureMessage())
        assertEquals(FailureMessage.Unexpected, Failure.Unexpected(409, "conflict").toFailureMessage())
        assertEquals(FailureMessage.Unexpected, Failure.Forbidden().toFailureMessage())
    }

    @Test
    fun somethingGoneIsWordedByTheServerUnlessTheScreenHasWords() {
        // Equipe has its own ("Esta pessoa não faz mais parte da equipe."); any other screen shows the API's.
        assertEquals(
            FailureMessage.Refused("Não encontrado"),
            Failure.NotFound(title = "Não encontrado").toFailureMessage(),
        )
        assertEquals(FailureMessage.Unexpected, Failure.NotFound().toFailureMessage())
    }

    @Test
    fun aServerFailureNeverShowsItsTitle() {
        assertEquals(
            FailureMessage.ServerUnavailable,
            Failure.Unexpected(503, "not_ready", "Indisponível").toFailureMessage(),
        )
        assertEquals(FailureMessage.ServerUnavailable, Failure.Unexpected(500, "internal").toFailureMessage())
    }

    @Test
    fun aFailureOnTheDeviceIsAGeneralError() {
        assertEquals(FailureMessage.Unexpected, Failure.Unexpected(0, "storage").toFailureMessage())
    }

    @Test
    fun failuresAScreenWordsItselfAreAGeneralErrorWhereItDoesNot() {
        // Forms mark fields for these; a screen without those fields has nothing better to say.
        val failures =
            listOf(
                Failure.InvalidInput(listOf(InputIssue("email", "Inválido"))),
                Failure.AlreadyTaken("slug"),
                Failure.WrongCode(),
                Failure.WrongCredentials(),
            )
        failures.forEach { assertEquals(FailureMessage.Unexpected, it.toFailureMessage(), it.message) }
    }

    @Test
    fun waitsUnderAMinuteStayInSeconds() {
        // Same thresholds as the web's errorMessage.ts, so both apps tell the same wait.
        assertNull(minutesToWait(1))
        assertNull(minutesToWait(59))
    }

    @Test
    fun longerWaitsRoundUpToWholeMinutes() {
        // Rounding down would send the user back before the limit resets.
        assertEquals(1, minutesToWait(60))
        assertEquals(2, minutesToWait(61))
        assertEquals(60, minutesToWait(3600))
    }
}
