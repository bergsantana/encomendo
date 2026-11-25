package br.com.encomendo.core.domain.session

/**
 * LogOut ends the session on this device; the app then shows the login.
 * It works offline (ADR 0016). Failures are thrown as `Failure`.
 */
class LogOut(
    private val auth: AuthRepository,
) {
    /** Logs out. */
    suspend operator fun invoke() = auth.logOut()
}
