package br.com.encomendo.core.domain.supplier

import br.com.encomendo.core.domain.catalog.Fetched
import br.com.encomendo.core.domain.paging.Page

// Thin on purpose, as the catalog use cases are.

/** ListSuppliers reads the store's suppliers, for the choice of "Fornecedor principal" and "Fornecedor reserva". */
class ListSuppliers(
    private val suppliers: SupplierRepository,
) {
    /** Returns up to [limit] suppliers by name from [cursor] on; null [cursor] reads the first page. */
    suspend operator fun invoke(
        cursor: String?,
        limit: Int,
    ): Fetched<Page<Supplier>> = suppliers.page(query = null, cursor = cursor, limit = limit)
}

/** CreateSupplier adds a supplier from "Novo fornecedor" in the item form (ADR 0025). */
class CreateSupplier(
    private val suppliers: SupplierRepository,
) {
    /** Creates the supplier [draft]; throws `Failure.AlreadyTaken` (field `name`) or `Failure.InvalidInput`. */
    suspend operator fun invoke(draft: SupplierDraft): Supplier = suppliers.create(draft)
}
