package br.com.encomendo.core.domain.session

/**
 * AuthRepository starts sessions on this device. A session it returns is
 * already stored, so [SessionRepository.session] emits it and the app opens
 * the management graph on its own.
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
}
