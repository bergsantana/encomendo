package br.com.encomendo.core.domain.session

/**
 * RenewSession brings the stored session up to date with the server, as the
 * web does on every load (ADR 0010): a role or store changed elsewhere then
 * shows without waiting for the next 401. It works offline: the session just
 * stays as it was.
 */
class RenewSession(
    private val sessions: SessionRepository,
) {
    /** Renews the session; never throws. */
    suspend operator fun invoke() = sessions.renew()
}
