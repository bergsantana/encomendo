package br.com.encomendo.core.network.tenancy

import br.com.encomendo.core.network.decoded
import io.ktor.client.HttpClient
import io.ktor.client.request.post
import io.ktor.client.request.setBody
import io.ktor.http.ContentType
import io.ktor.http.contentType
import kotlinx.serialization.Serializable

/**
 * TenancyApi calls the stores' endpoints (docs/api/openapi.yaml, `tenancy`).
 * Calls carry the session's access token; an answer that isn't 2xx, or a 2xx
 * that isn't what the contract says, throws ApiError.
 */
class TenancyApi(
    private val client: HttpClient,
) {
    /**
     * Creates a store with [name] at `{slug}.encomendo.com.br`, owned by the
     * caller, with or without a store selected. The session does not change:
     * switch to the returned store to open it.
     */
    suspend fun createTenant(
        name: String,
        slug: String,
    ): TenantDto =
        client
            .post("/api/v1/admin/tenants") {
                contentType(ContentType.Application.Json)
                setBody(CreateTenantRequest(name = name, slug = slug))
            }.decoded()
}

/** TenantDto is a store as `POST /api/v1/admin/tenants` returns it. Not a data class, so logs never carry its name. */
@Serializable
class TenantDto(
    val id: String,
    val name: String,
    val slug: String,
)

@Serializable
private class CreateTenantRequest(
    val name: String,
    val slug: String,
)
