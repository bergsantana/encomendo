package br.com.encomendo.core.network.catalog

import io.ktor.client.HttpClient

/**
 * CatalogApis holds the catalog's API classes over one client, so the
 * repository takes one input instead of four.
 */
class CatalogApis(
    client: HttpClient,
) {
    /** Categories and item fields. */
    val categories = CategoriesApi(client)

    /** Items. */
    val items = ItemsApi(client)

    /** Kits. */
    val kits = KitsApi(client)

    /** The photo calls of items and kits, except the upload to storage. */
    val photos = PhotosApi(client)
}
