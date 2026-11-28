import type { HttpClient } from './http'
import { type PageRequest, queryString } from './query'
import type { components } from './types/schema'

type Schemas = components['schemas']

/** Customer is one customer of the store with the addresses. A customer is never shared with another store. */
export type Customer = Schemas['Customer']
/** CustomerInput is the body of POST /api/v1/admin/customers. */
export type CustomerInput = Schemas['CustomerInput']
/** ReplaceCustomerRequest is CustomerInput plus the `version` that was read. */
export type ReplaceCustomerRequest = Schemas['ReplaceCustomerRequest']
/** CustomerSummary is one row of the customer list. */
export type CustomerSummary = Schemas['CustomerSummary']
/** CustomerPage is one page of customers, by name. */
export type CustomerPage = Schemas['CustomerPage']
/** Address is one of a customer's addresses. */
export type Address = Schemas['Address']
/** AddressInput is an address in a save: with its `id` it is kept, without it a new one is made. */
export type AddressInput = Schemas['AddressInput']
/** UF is a Brazilian state code. */
export type UF = Schemas['UF']

/** CustomerListRequest asks for one page of customers, optionally those whose name starts with `q` or whose WhatsApp has its digits. */
export interface CustomerListRequest extends PageRequest {
  q?: string
}

/** listCustomers calls GET /api/v1/admin/customers (`order:read`). */
export function listCustomers(client: HttpClient, request: CustomerListRequest = {}): Promise<CustomerPage> {
  const query = queryString({ cursor: request.cursor, limit: request.limit, q: request.q })
  return client.get<CustomerPage>(`/api/v1/admin/customers${query}`)
}

/** getCustomer calls GET /api/v1/admin/customers/{customerId}; an unknown id throws `not_found`. */
export function getCustomer(client: HttpClient, customerId: string): Promise<Customer> {
  return client.get<Customer>(`/api/v1/admin/customers/${encodeURIComponent(customerId)}`)
}

/**
 * createCustomer calls POST /api/v1/admin/customers ("Novo cliente"). A
 * WhatsApp number another customer already has throws `phone_taken`.
 */
export function createCustomer(client: HttpClient, body: CustomerInput): Promise<Customer> {
  return client.post<Customer>('/api/v1/admin/customers', body)
}

/**
 * updateCustomer calls PUT /api/v1/admin/customers/{customerId}
 * (`customer:write`). Send every field and address with the `version` read;
 * an address left out is removed. Throws `phone_taken` and `version_conflict`.
 */
export function updateCustomer(client: HttpClient, customerId: string, body: ReplaceCustomerRequest): Promise<Customer> {
  return client.put<Customer>(`/api/v1/admin/customers/${encodeURIComponent(customerId)}`, body)
}
