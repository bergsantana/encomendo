package br.com.encomendo.core.testing

import br.com.encomendo.core.domain.session.AuthRepository
import br.com.encomendo.core.domain.session.Session

/**
 * FakeAuthRepository records every login and logout. [answer] returns a
 * session or throws a `Failure`; [logOutAnswer] returns or throws. A test
 * that needs a call to hang makes it wait on a `CompletableDeferred`.
 */
class FakeAuthRepository(
    var answer: suspend (email: String, password: String) -> Session = { _, _ -> testSession() },
    var logOutAnswer: suspend () -> Unit = {},
) : AuthRepository {
    private val seen = mutableListOf<Pair<String, String>>()

    /** The (email, password) pairs received so far, in order. */
    val logins: List<Pair<String, String>> get() = seen.toList()

    /** How many times logOut was called. */
    var logouts: Int = 0
        private set

    override suspend fun logIn(
        email: String,
        password: String,
    ): Session {
        seen += email to password
        return answer(email, password)
    }

    override suspend fun logOut() {
        logouts++
        logOutAnswer()
    }
}
