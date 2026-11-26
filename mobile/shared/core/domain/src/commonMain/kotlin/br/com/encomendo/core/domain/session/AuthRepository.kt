package br.com.encomendo.core.domain.session

/**
 * AuthRepository starts and ends sessions on this device. A session it
 * returns is already stored, so [SessionRepository.session] emits it and the
 * app opens the management graph on its own; after a logout it emits null.
 */
interface AuthRepository {
    /**
     * Logs in with [email] and [password] and keeps the new session. Throws
     * `Failure`: [br.com.encomendo.core.domain.failure.Failure.WrongCredentials]
     * when they match no account, and nothing is stored on any failure.
     */
    suspend fun logIn(
        email: String,
        password: String,
    ): Session

    /**
     * Creates a staff account and keeps its session, which has no store yet:
     * the app then offers to create one. Throws `Failure`:
     * [br.com.encomendo.core.domain.failure.Failure.AlreadyTaken] on the
     * `email` field when the email has an account already.
     */
    suspend fun signUp(
        name: String,
        email: String,
        password: String,
    ): Session

    /**
     * Forgets the session on this device at once, then asks the server to end
     * it, without waiting: leaving works offline. Throws `Failure` only when
     * the device cannot forget it; the session then stays as it was.
     */
    suspend fun logOut()
}
