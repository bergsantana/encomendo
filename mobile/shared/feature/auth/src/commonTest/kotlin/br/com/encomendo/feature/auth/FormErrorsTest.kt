package br.com.encomendo.feature.auth

import br.com.encomendo.core.domain.failure.Failure
import kotlin.test.Test
import kotlin.test.assertEquals

class FormErrorsTest {
    @Test
    fun aRefusalShowsTheServersOwnWords() {
        assertEquals(
            FormError.Refused("Conta bloqueada"),
            Failure.Unexpected(409, "conflict", "Conta bloqueada").toFormError(),
        )
        assertEquals(FormError.Refused("Sem permissão"), Failure.Forbidden(title = "Sem permissão").toFormError())
    }

    @Test
    fun aRefusalWithoutWordsIsAGeneralError() {
        // Never an empty banner above the form.
        assertEquals(FormError.Unexpected, Failure.Unexpected(409, "conflict", title = "  ").toFormError())
        assertEquals(FormError.Unexpected, Failure.Unexpected(409, "conflict").toFormError())
        assertEquals(FormError.Unexpected, Failure.Forbidden().toFormError())
    }

    @Test
    fun aServerFailureNeverShowsItsTitle() {
        assertEquals(FormError.ServerUnavailable, Failure.Unexpected(503, "not_ready", "Indisponível").toFormError())
    }
}
