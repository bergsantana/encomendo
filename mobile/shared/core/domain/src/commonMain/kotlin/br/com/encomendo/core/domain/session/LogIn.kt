package br.com.encomendo.core.domain.session

/**
 * LogIn signs a business user in with email and password. The email is
 * trimmed, because keyboards add a space after an autocompleted address; the
 * password is sent exactly as typed. Failures are thrown as `Failure`.
 */
class LogIn(
    private val auth: AuthRepository,
) {
    /** Logs in and returns the stored session. */
    suspend operator fun invoke(
        email: String,
        password: String,
    ): Session = auth.logIn(email = email.trim(), password = password)
}
