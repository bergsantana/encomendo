import type { HttpClient } from './http'
import type { components } from './types/schema'

export type Status = components['schemas']['Status']

/**
 * getReadiness calls GET /readyz; it throws ApiError with code "not_ready"
 * when a dependency is down. The probe is public, so it never carries the
 * access token.
 */
export function getReadiness(client: HttpClient): Promise<Status> {
  return client.get<Status>('/readyz', { anonymous: true })
}
