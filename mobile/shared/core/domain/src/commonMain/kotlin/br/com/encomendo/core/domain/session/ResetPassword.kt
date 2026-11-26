package br.com.encomendo.core.domain.session

/**
 * ResetPassword sets a new password with the emailed code. The code loses
 * its spaces, since one copied from the email may come as "482 913"; the
 * password is sent exactly as typed. Failures are thrown as `Failure`.
 */
class ResetPassword(
    private val resets: PasswordResetRepository,
) {
    /** Sets the new password. */
    suspend operator fun invoke(
        email: String,
        code: String,
        password: String,
    ) = resets.reset(email = email.trim(), code = code.withoutSpaces(), password = password)

    /** The code's rule, checked before sending as the web does. */
    companion object {
        /** Whether [code], spaces aside, is 6 digits, as the contract's `PasswordResetConfirmRequest.code` requires. */
        fun isWellFormed(code: String): Boolean = SIX_DIGITS.matches(code.withoutSpaces())
    }
}

private val SIX_DIGITS = Regex("^[0-9]{6}$")

private fun String.withoutSpaces(): String = filterNot(Char::isWhitespace)
