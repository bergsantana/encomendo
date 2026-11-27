package br.com.encomendo.core.network.catalog

import br.com.encomendo.core.network.decoded
import io.ktor.client.HttpClient
import io.ktor.client.request.get
import io.ktor.client.request.parameter
import io.ktor.client.request.post
import io.ktor.client.request.put
import io.ktor.http.encodeURLPathPart

/**
 * SuppliersApi calls the suppliers' endpoints (docs/api/openapi.yaml,
 * `suppliers`). Calls carry the session's access token; reading needs
 * `catalog:read` and writing `catalog:write`. A supplier carries no cost.
 */
class SuppliersApi(
    private val client: HttpClient,
) {
    /** Returns up to [limit] suppliers by name from [cursor] on; null [cursor] reads the first page. */
    suspend fun list(
        query: String?,
        cursor: String?,
        limit: Int,
    ): SupplierPageDto =
        client
            .get(SUPPLIERS) {
                parameter("q", query)
                parameter("cursor", cursor)
                parameter("limit", limit)
            }.decoded()

    /** Returns the supplier [id]. */
    suspend fun get(id: String): SupplierDto = client.get(supplierPath(id)).decoded()

    /** Adds a supplier. */
    suspend fun create(body: SupplierWriteDto): SupplierDto = client.post(SUPPLIERS) { jsonBody(body) }.decoded()

    /** Replaces the supplier [id]; [body] must carry the `version` read. */
    suspend fun replace(
        id: String,
        body: SupplierWriteDto,
    ): SupplierDto = client.put(supplierPath(id)) { jsonBody(body) }.decoded()

    private fun supplierPath(id: String): String = "$SUPPLIERS/${id.encodeURLPathPart()}"

    private companion object {
        const val SUPPLIERS = "/api/v1/admin/suppliers"
    }
}
