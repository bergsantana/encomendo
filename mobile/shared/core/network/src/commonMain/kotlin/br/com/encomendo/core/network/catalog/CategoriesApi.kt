package br.com.encomendo.core.network.catalog

import br.com.encomendo.core.network.decoded
import io.ktor.client.HttpClient
import io.ktor.client.request.delete
import io.ktor.client.request.get
import io.ktor.client.request.patch
import io.ktor.client.request.post
import io.ktor.client.request.put
import io.ktor.http.encodeURLPathPart
import kotlinx.serialization.Serializable

/** The catalog's route, which every catalog call here starts from. */
internal const val CATALOG = "/api/v1/admin/catalog"

/**
 * CategoriesApi calls the categories' and item fields' endpoints
 * (docs/api/openapi.yaml, `catalog`). Calls carry the session's access
 * token; reading needs `catalog:read` and writing `catalog:write`. An answer
 * that isn't 2xx, or a 2xx that isn't what the contract says, throws ApiError.
 */
class CategoriesApi(
    private val client: HttpClient,
) {
    /** Returns every category in the store's order. */
    suspend fun categories(): CategoryListDto = client.get(CATEGORIES).decoded()

    /** Adds a category at the end of the list. */
    suspend fun createCategory(name: String): CategoryDto =
        client.post(CATEGORIES) { jsonBody(CategoryWriteDto(name = name)) }.decoded()

    /** Renames or moves the category [id]; send [name], [position] or both. */
    suspend fun updateCategory(
        id: String,
        name: String?,
        position: Int?,
    ): CategoryDto =
        client
            .patch("$CATEGORIES/${id.encodeURLPathPart()}") { jsonBody(CategoryUpdateDto(name, position)) }
            .decoded()

    /** Deletes the category [id]; the answer is 204 with no body. */
    suspend fun deleteCategory(id: String) {
        client.delete("$CATEGORIES/${id.encodeURLPathPart()}")
    }

    /** Returns every item field of the store with its values. */
    suspend fun attributes(): AttributeListDto = client.get(ATTRIBUTES).decoded()

    /** Adds an item field with its values. */
    suspend fun createAttribute(body: AttributeWriteDto): AttributeDto =
        client.post(ATTRIBUTES) { jsonBody(body) }.decoded()

    /** Replaces the field [id]'s name and values. */
    suspend fun replaceAttribute(
        id: String,
        body: AttributeWriteDto,
    ): AttributeDto = client.put("$ATTRIBUTES/${id.encodeURLPathPart()}") { jsonBody(body) }.decoded()

    /** Deletes the field [id]; the answer is 204 with no body. */
    suspend fun deleteAttribute(id: String) {
        client.delete("$ATTRIBUTES/${id.encodeURLPathPart()}")
    }

    private companion object {
        const val CATEGORIES = "$CATALOG/categories"
        const val ATTRIBUTES = "$CATALOG/attributes"
    }
}

@Serializable
private class CategoryWriteDto(
    val name: String,
)

@Serializable
private class CategoryUpdateDto(
    // Defaults, so a field the caller left out is not sent as null: the API refuses a null name.
    val name: String? = null,
    val position: Int? = null,
)
