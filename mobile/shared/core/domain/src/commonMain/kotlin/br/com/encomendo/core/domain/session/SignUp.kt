package br.com.encomendo.core.domain.session

/**
 * SignUp creates a staff account and signs it in, without a store yet. The
 * name and email are trimmed, because keyboards add a space after
 * autocomplete; the password is sent exactly as typed. Failures are thrown as
 * `Failure`.
 */
class SignUp(
    private val auth: AuthRepository,
) {
    /** Creates the account and returns its stored session. */
    suspend operator fun invoke(
        name: String,
        email: String,
        password: String,
    ): Session = auth.signUp(name = name.trim(), email = email.trim(), password = password)
}
