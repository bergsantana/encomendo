package br.com.encomendo.core.network.catalog

import br.com.encomendo.core.network.decoded
import io.ktor.client.HttpClient
import io.ktor.client.request.get
import io.ktor.client.request.parameter
import io.ktor.client.request.post
import io.ktor.client.request.put
import io.ktor.http.encodeURLPathPart
import kotlinx.serialization.Serializable

/**
 * ItemsApi calls the catalog items' endpoints (docs/api/openapi.yaml,
 * `catalog`). Calls carry the session's access token; reading needs
 * `catalog:read` and writing `catalog:write`.
 *
 * Costs and margins decode into [OwnerFieldDto], so a field the API left out
 * stays different from one it sent as null. A write body carries a cost field
 * only when the caller set it (ADR 0025).
 */
class ItemsApi(
    private val client: HttpClient,
) {
    /** Returns up to [limit] items by name from [cursor] on; null [cursor] reads the first page. */
    suspend fun list(
        query: String?,
        categoryId: String?,
        includeArchived: Boolean,
        cursor: String?,
        limit: Int,
    ): ItemPageDto =
        client
            .get(ITEMS) {
                parameter("q", query)
                parameter("categoryId", categoryId)
                // The default is false, so the parameter is sent only when it matters.
                if (includeArchived) parameter("includeArchived", true)
                parameter("cursor", cursor)
                parameter("limit", limit)
            }.decoded()

    /** Returns the item [id]. */
    suspend fun get(id: String): ItemDto = client.get(itemPath(id)).decoded()

    /** Adds an item. */
    suspend fun create(body: ItemWriteDto): ItemDto = client.post(ITEMS) { jsonBody(body) }.decoded()

    /** Replaces the item [id]; [body] must carry the `version` read. */
    suspend fun replace(
        id: String,
        body: ItemWriteDto,
    ): ItemDto = client.put(itemPath(id)) { jsonBody(body) }.decoded()

    /** Shows or hides the item on the store page. */
    suspend fun setPublished(
        id: String,
        published: Boolean,
        version: Int,
    ): ItemDto = client.put("${itemPath(id)}/published") { jsonBody(PublishedDto(published, version)) }.decoded()

    /** Archives the item; the answer is the archived item. */
    suspend fun archive(
        id: String,
        version: Int,
    ): ItemDto = client.post("${itemPath(id)}/archive") { jsonBody(VersionDto(version)) }.decoded()

    /** Restores an archived item, unpublished. */
    suspend fun restore(
        id: String,
        version: Int,
    ): ItemDto = client.post("${itemPath(id)}/restore") { jsonBody(VersionDto(version)) }.decoded()

    // Ids are encoded, so one can only ever name one row, never another path.
    private fun itemPath(id: String): String = "$ITEMS/${id.encodeURLPathPart()}"

    private companion object {
        const val ITEMS = "$CATALOG/items"
    }
}

/** The body of `PUT .../published` on an item or kit. */
@Serializable
internal class PublishedDto(
    val published: Boolean,
    val version: Int,
)

/** The body of `POST .../archive` and `.../restore` on an item or kit. */
@Serializable
internal class VersionDto(
    val version: Int,
)
