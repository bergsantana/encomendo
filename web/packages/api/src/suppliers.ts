import type { HttpClient } from './http'
import { type PageRequest, queryString } from './query'
import type { components } from './types/schema'

type Schemas = components['schemas']

/** Supplier is one of the store's suppliers ("Fornecedores"). It carries no cost. */
export type Supplier = Schemas['Supplier']
/** SupplierPage is one page of suppliers, by name. */
export type SupplierPage = Schemas['SupplierPage']
/** SupplierInput is the body of POST /api/v1/admin/suppliers. */
export type SupplierInput = Schemas['SupplierInput']
/** ReplaceSupplierRequest is the body of PUT /api/v1/admin/suppliers/{supplierId}: the input plus the `version` read. */
export type ReplaceSupplierRequest = Schemas['ReplaceSupplierRequest']

/** SupplierItemUse is one catalog item a supplier supplies, as main or backup, with its lead time. Its `cost` is the owner's only. */
export type SupplierItemUse = Schemas['SupplierItemUse']
/** SupplierItemUseList is the items a supplier supplies. */
export type SupplierItemUseList = Schemas['SupplierItemUseList']

/** SupplierListRequest asks for one page of suppliers, optionally only those whose name contains `q`. */
export interface SupplierListRequest extends PageRequest {
  q?: string
}

// Reading needs `catalog:read` (every role); writing needs `catalog:write`.
// Suppliers are staff data: the store page never receives them.

/** listSuppliers calls GET /api/v1/admin/suppliers. */
export function listSuppliers(client: HttpClient, request: SupplierListRequest = {}): Promise<SupplierPage> {
  const query = queryString({ cursor: request.cursor, limit: request.limit, q: request.q })
  return client.get<SupplierPage>(`/api/v1/admin/suppliers${query}`)
}

/** createSupplier calls POST /api/v1/admin/suppliers ("Novo fornecedor"). */
export function createSupplier(client: HttpClient, body: SupplierInput): Promise<Supplier> {
  return client.post<Supplier>('/api/v1/admin/suppliers', body)
}

/** getSupplier calls GET /api/v1/admin/suppliers/{supplierId}; an unknown id throws `not_found`. */
export function getSupplier(client: HttpClient, supplierId: string): Promise<Supplier> {
  return client.get<Supplier>(`/api/v1/admin/suppliers/${encodeURIComponent(supplierId)}`)
}

/**
 * replaceSupplier calls PUT /api/v1/admin/suppliers/{supplierId}. The body
 * carries the `version` that was read; someone saving first throws
 * `version_conflict`.
 */
export function replaceSupplier(client: HttpClient, supplierId: string, body: ReplaceSupplierRequest): Promise<Supplier> {
  return client.put<Supplier>(`/api/v1/admin/suppliers/${encodeURIComponent(supplierId)}`, body)
}

/** supplierItems calls GET /api/v1/admin/suppliers/{supplierId}/items: the catalog items the supplier supplies, for the Fornecedores tab. Archived items are left out. */
export function supplierItems(client: HttpClient, supplierId: string): Promise<SupplierItemUseList> {
  return client.get<SupplierItemUseList>(`/api/v1/admin/suppliers/${encodeURIComponent(supplierId)}/items`)
}
