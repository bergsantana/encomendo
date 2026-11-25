import type { HttpClient } from './http'
import type { components } from './types/schema'

type Schemas = components['schemas']

/** AuthSession is what every session-starting call returns (ADR 0010): the access token plus the Session. */
export type AuthSession = Schemas['AuthSession']
/** Session is the user, the selected store (if any), the role and permissions there, and every membership. */
export type Session = Schemas['Session']
/** Permission names what a role may do in a store; the backend owns the role → permission map. */
export type Permission = Schemas['Permission']
/** Role is a staff member's role in one store. */
export type Role = Schemas['Role']
/** TenantSummary is a store as the session and the store picker show it. */
export type TenantSummary = Schemas['TenantSummary']
/** Membership is one store the user belongs to, with the role there. */
export type Membership = Schemas['Membership']
/** User is the signed-in staff member. */
export type User = Schemas['User']
/** LoginRequest is the body of POST /auth/login. */
export type LoginRequest = Schemas['LoginRequest']

// Every call here is anonymous: /auth authenticates with a password or the
// refresh cookie, never the access token, and a 401 from it must not trigger
// a refresh (ADR 0010, ADR 0013).

/**
 * logIn calls POST /auth/login. The refresh token arrives as an httpOnly
 * cookie. A wrong email or password throws ApiError `invalid_credentials`.
 */
export function logIn(client: HttpClient, body: LoginRequest): Promise<AuthSession> {
  return client.post<AuthSession>('/auth/login', body, { anonymous: true })
}

/**
 * refreshSession calls POST /auth/refresh, which reads the refresh cookie and
 * rotates it. A refusal (400, 401, 403) means the session is over.
 */
export function refreshSession(client: HttpClient): Promise<AuthSession> {
  return client.post<AuthSession>('/auth/refresh', {}, { anonymous: true })
}

/**
 * logOut calls POST /auth/logout, which revokes the refresh cookie's family
 * and always answers 204. Passing the access token makes the API stop it at
 * once instead of at its expiry.
 */
export async function logOut(client: HttpClient, accessToken: string | null): Promise<void> {
  const headers: Record<string, string> = accessToken === null ? {} : { Authorization: `Bearer ${accessToken}` }
  await client.post<unknown>('/auth/logout', {}, { anonymous: true, headers })
}
