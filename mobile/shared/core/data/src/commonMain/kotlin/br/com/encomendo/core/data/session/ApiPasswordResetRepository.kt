package br.com.encomendo.core.data.session

import br.com.encomendo.core.data.failure.asFailures
import br.com.encomendo.core.domain.session.PasswordResetRepository
import br.com.encomendo.core.network.auth.AuthApi

/** ApiPasswordResetRepository resets a forgotten password through the `/auth/password-reset` endpoints. */
class ApiPasswordResetRepository(
    private val api: AuthApi,
) : PasswordResetRepository {
    override suspend fun requestCode(email: String) = asFailures { api.requestPasswordReset(email) }

    override suspend fun reset(
        email: String,
        code: String,
        password: String,
    ) = asFailures { api.confirmPasswordReset(email = email, code = code, password = password) }
}
