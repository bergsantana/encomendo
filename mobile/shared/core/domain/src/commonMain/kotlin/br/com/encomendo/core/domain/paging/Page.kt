package br.com.encomendo.core.domain.paging

/**
 * Page is one page of a list the API reads a page at a time (`cursor` and
 * `limit` in docs/api/openapi.yaml). [nextCursor] is where the next page
 * starts, and null on the last page.
 */
data class Page<T>(
    /** The items on this page, in the list's order. */
    val items: List<T>,
    /** Where the next page starts, or null when this is the last. */
    val nextCursor: String?,
)
