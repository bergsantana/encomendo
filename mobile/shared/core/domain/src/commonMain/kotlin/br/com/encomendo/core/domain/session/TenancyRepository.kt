package br.com.encomendo.core.domain.session

/**
 * TenancyRepository creates stores and moves the session between them. A
 * session it returns is already stored, so [SessionRepository.session] emits
 * it and the app opens that store on its own.
 */
interface TenancyRepository {
    /**
     * Spends the refresh token for a session in [tenantId] and keeps it, even
     * when the caller is cancelled once the request is sent. On a refusal
     * (`Failure.Forbidden`: no longer a member) the session stays as it was
     * (ADR 0013). A 401 is the refresh token itself refused, which a refresh
     * would only confirm, so the session ends at once (ADR 0012).
     */
    suspend fun switchTo(tenantId: String): Session

    /**
     * Creates a store named [name] at the address [slug], owned by the
     * signed-in user. The session does not move to it: call [switchTo].
     * Throws `Failure.AlreadyTaken` when another store has that address.
     */
    suspend fun createStore(
        name: String,
        slug: String,
    ): TenantSummary
}
