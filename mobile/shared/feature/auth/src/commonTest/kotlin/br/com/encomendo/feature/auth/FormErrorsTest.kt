package br.com.encomendo.feature.auth

import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.domain.failure.InputIssue
import br.com.encomendo.core.ui.failure.FailureMessage
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

class FormErrorsTest {
    @Test
    fun aRefusedFieldAsksToCheckTheFields() {
        assertEquals(FormError.CheckFields, Failure.InvalidInput(listOf(InputIssue("email", "Inválido"))).toFormError())
        assertEquals(FormError.CheckFields, Failure.AlreadyTaken("slug").toFormError())
        assertEquals(FormError.CheckFields, Failure.WrongCode().toFormError())
    }

    @Test
    fun wrongCredentialsHaveTheLoginsWords() {
        assertEquals(FormError.WrongCredentials, Failure.WrongCredentials().toFormError())
    }

    @Test
    fun anyOtherFailureHasTheWordsEveryScreenUses() {
        // core:ui's FailureMessageTest covers each message; here only that forms pass them on.
        assertEquals(FormError.General(FailureMessage.Offline), Failure.Offline().toFormError())
        assertEquals(
            FormError.General(FailureMessage.Refused("Sem permissão")),
            Failure.Forbidden(title = "Sem permissão").toFormError(),
        )
        assertEquals(
            FormError.General(FailureMessage.ServerUnavailable),
            Failure.Unexpected(503, "not_ready", "Indisponível").toFormError(),
        )
    }

    @Test
    fun typingKeepsOnlyARateLimitOrTheMarksStillShown() {
        val rateLimited = FormError.General(FailureMessage.RateLimited(30))
        assertEquals(rateLimited, rateLimited.afterTyping(stillMarked = null), "typing doesn't shorten the wait")
        assertEquals(FormError.CheckFields, FormError.CheckFields.afterTyping(stillMarked = FieldProblem.Missing))
        assertNull(FormError.CheckFields.afterTyping(stillMarked = null))
        assertNull(FormError.General(FailureMessage.Offline).afterTyping(stillMarked = FieldProblem.Missing))
    }
}
