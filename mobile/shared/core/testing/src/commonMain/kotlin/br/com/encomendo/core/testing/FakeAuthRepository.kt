package br.com.encomendo.core.testing

import br.com.encomendo.core.domain.session.AuthRepository
import br.com.encomendo.core.domain.session.Session

/**
 * FakeAuthRepository records every login and answers with [answer], which
 * returns a session or throws a `Failure`. A test that needs a login to hang
 * makes [answer] wait on a `CompletableDeferred`.
 */
class FakeAuthRepository(
    var answer: suspend (email: String, password: String) -> Session = { _, _ -> testSession() },
) : AuthRepository {
    private val seen = mutableListOf<Pair<String, String>>()

    /** The (email, password) pairs received so far, in order. */
    val logins: List<Pair<String, String>> get() = seen.toList()

    override suspend fun logIn(
        email: String,
        password: String,
    ): Session {
        seen += email to password
        return answer(email, password)
    }
}
