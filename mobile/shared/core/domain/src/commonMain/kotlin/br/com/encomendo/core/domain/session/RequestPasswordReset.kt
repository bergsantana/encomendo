package br.com.encomendo.core.domain.session

/** RequestPasswordReset emails a code to the trimmed [email][invoke]. Failures are thrown as `Failure`. */
class RequestPasswordReset(
    private val resets: PasswordResetRepository,
) {
    /** Asks for the code. */
    suspend operator fun invoke(email: String) = resets.requestCode(email.trim())
}
