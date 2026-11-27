package br.com.encomendo.core.domain.supplier

import br.com.encomendo.core.domain.catalog.Fetched
import br.com.encomendo.core.domain.paging.Page

/**
 * Supplier is a supplier of the store (`Supplier` in docs/api/openapi.yaml). Staff data: it never reaches the
 * store page, and it carries no cost.
 */
data class Supplier(
    /** The supplier's id (a UUID). */
    val id: String,
    /** The supplier's name. */
    val name: String,
    /** Digits only, 55 + DDD + number, as wa.me links need it. */
    val whatsappPhone: String,
    /** The version to send back when saving. */
    val version: Int,
)

/** SupplierDraft is a supplier as the user typed it, to create or replace. */
data class SupplierDraft(
    /** The supplier's name. */
    val name: String,
    /** Digits only, 55 + DDD + number. */
    val whatsappPhone: String,
)

/**
 * SupplierRepository reads and writes the store's suppliers. Reading needs
 * `catalog:read` and writing `catalog:write` (ADR 0025); a call the role may
 * not make throws `Failure.Forbidden`. Reads fall back to the copy kept on the
 * device when the call cannot reach the server, and say so in [Fetched].
 */
interface SupplierRepository {
    /** Returns up to [limit] suppliers by name from [cursor] on (null for the first page), filtered by [query]. */
    suspend fun page(
        query: String?,
        cursor: String?,
        limit: Int,
    ): Fetched<Page<Supplier>>

    /** Returns the supplier [id]. Throws `Failure.NotFound` when it is gone. */
    suspend fun supplier(id: String): Fetched<Supplier>

    /** Creates a supplier and returns it. Throws `Failure.InvalidInput` with the fields the API refused. */
    suspend fun create(draft: SupplierDraft): Supplier

    /**
     * Replaces the supplier's name and WhatsApp, given the [version] read; throws `Failure.VersionConflict` when
     * someone saved first.
     */
    suspend fun replace(
        id: String,
        version: Int,
        draft: SupplierDraft,
    ): Supplier
}
