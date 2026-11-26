package br.com.encomendo.core.domain.session

/**
 * PasswordResetRepository resets a forgotten password with a code sent by
 * email ("Esqueci a senha", ADR 0011). It signs nobody in: the user logs in
 * with the new password afterwards.
 */
interface PasswordResetRepository {
    /**
     * Asks for a 6-digit code for [email]. Succeeds the same whether or not
     * the email has an account, so the app never tells who is registered.
     * A new code replaces the earlier one.
     */
    suspend fun requestCode(email: String)

    /**
     * Sets [password] with [code]. Every session of the account ends. Throws
     * `Failure.WrongCode` for a wrong, expired or used-up code.
     */
    suspend fun reset(
        email: String,
        code: String,
        password: String,
    )
}
