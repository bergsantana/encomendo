package br.com.encomendo.core.network.catalog

import br.com.encomendo.core.network.decoded
import io.ktor.client.HttpClient
import io.ktor.client.request.get
import io.ktor.client.request.parameter
import io.ktor.client.request.post
import io.ktor.client.request.put
import io.ktor.http.encodeURLPathPart

/**
 * KitsApi calls the kits' endpoints (docs/api/openapi.yaml, `catalog`).
 * Calls carry the session's access token; reading needs `catalog:read` and
 * writing `catalog:write`. Costs and margins decode into [OwnerFieldDto] as
 * on [ItemsApi].
 */
class KitsApi(
    private val client: HttpClient,
) {
    /** Returns up to [limit] kits by name from [cursor] on; null [cursor] reads the first page. */
    suspend fun list(
        query: String?,
        includeArchived: Boolean,
        cursor: String?,
        limit: Int,
    ): KitPageDto =
        client
            .get(KITS) {
                parameter("q", query)
                if (includeArchived) parameter("includeArchived", true)
                parameter("cursor", cursor)
                parameter("limit", limit)
            }.decoded()

    /** Returns the kit [id]. */
    suspend fun get(id: String): KitDto = client.get(kitPath(id)).decoded()

    /** Adds a kit. */
    suspend fun create(body: KitWriteDto): KitDto = client.post(KITS) { jsonBody(body) }.decoded()

    /** Replaces the kit [id]; [body] must carry the `version` read. */
    suspend fun replace(
        id: String,
        body: KitWriteDto,
    ): KitDto = client.put(kitPath(id)) { jsonBody(body) }.decoded()

    /** Shows or hides the kit on the store page. */
    suspend fun setPublished(
        id: String,
        published: Boolean,
        version: Int,
    ): KitDto = client.put("${kitPath(id)}/published") { jsonBody(PublishedDto(published, version)) }.decoded()

    /** Archives the kit. */
    suspend fun archive(
        id: String,
        version: Int,
    ): KitDto = client.post("${kitPath(id)}/archive") { jsonBody(VersionDto(version)) }.decoded()

    /** Restores an archived kit, unpublished. */
    suspend fun restore(
        id: String,
        version: Int,
    ): KitDto = client.post("${kitPath(id)}/restore") { jsonBody(VersionDto(version)) }.decoded()

    private fun kitPath(id: String): String = "$KITS/${id.encodeURLPathPart()}"

    private companion object {
        const val KITS = "$CATALOG/kits"
    }
}
