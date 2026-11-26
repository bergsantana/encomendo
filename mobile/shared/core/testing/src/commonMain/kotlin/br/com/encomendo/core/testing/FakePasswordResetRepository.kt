package br.com.encomendo.core.testing

import br.com.encomendo.core.domain.session.PasswordResetRepository

/**
 * FakePasswordResetRepository records every code asked for and every reset.
 * [requestAnswer] and [resetAnswer] return or throw a `Failure`.
 */
class FakePasswordResetRepository(
    var requestAnswer: suspend (email: String) -> Unit = {},
    var resetAnswer: suspend (email: String, code: String, password: String) -> Unit = { _, _, _ -> },
) : PasswordResetRepository {
    private val asked = mutableListOf<String>()
    private val done = mutableListOf<Triple<String, String, String>>()

    /** The emails a code was asked for, in order. */
    val requested: List<String> get() = asked.toList()

    /** The (email, code, password) resets received, in order. */
    val resets: List<Triple<String, String, String>> get() = done.toList()

    override suspend fun requestCode(email: String) {
        asked += email
        requestAnswer(email)
    }

    override suspend fun reset(
        email: String,
        code: String,
        password: String,
    ) {
        done += Triple(email, code, password)
        resetAnswer(email, code, password)
    }
}
