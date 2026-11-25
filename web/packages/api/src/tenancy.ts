import type { HttpClient } from './http'
import type { components } from './types/schema'

/** Tenant is a store as the API returns it after creating it. */
export type Tenant = components['schemas']['Tenant']
/** CreateTenantRequest is the body of POST /api/v1/admin/tenants. */
export type CreateTenantRequest = components['schemas']['CreateTenantRequest']

/**
 * createTenant calls POST /api/v1/admin/tenants; the caller becomes the
 * owner. The session does not change: switch to the new store afterwards.
 * An address already in use throws `slug_taken`.
 */
export function createTenant(client: HttpClient, body: CreateTenantRequest): Promise<Tenant> {
  return client.post<Tenant>('/api/v1/admin/tenants', body)
}
