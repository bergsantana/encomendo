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
/** SignUpRequest is the body of POST /auth/signup. */
export type SignUpRequest = Schemas['SignUpRequest']
/** PasswordResetConfirmRequest is the body of POST /auth/password-reset/confirm. */
export type PasswordResetConfirmRequest = Schemas['PasswordResetConfirmRequest']
/** InvitationPreview is what an invitation link offers: the store, the email, the role and whether that email has an account. */
export type InvitationPreview = Schemas['InvitationPreview']
/** AcceptInvitationRequest is the body of POST /auth/invitations/accept. */
export type AcceptInvitationRequest = Schemas['AcceptInvitationRequest']

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
 * signUp calls POST /auth/signup. The new session has no store: create one,
 * then switch to it. An email already registered throws `email_taken`.
 */
export function signUp(client: HttpClient, body: SignUpRequest): Promise<AuthSession> {
  return client.post<AuthSession>('/auth/signup', body, { anonymous: true })
}

/**
 * switchTenant calls POST /auth/switch-tenant, which rotates the refresh
 * cookie into a session for that store. A store the user does not belong to
 * throws `forbidden` and leaves the session as it was (ADR 0013).
 */
export function switchTenant(client: HttpClient, tenantId: string): Promise<AuthSession> {
  return client.post<AuthSession>('/auth/switch-tenant', { tenantId }, { anonymous: true })
}

/**
 * requestPasswordReset calls POST /auth/password-reset, which emails a
 * 6-digit code when the account exists. It always answers 202, so the answer
 * never tells who is registered (ADR 0011).
 */
export async function requestPasswordReset(client: HttpClient, email: string): Promise<void> {
  await client.post<unknown>('/auth/password-reset', { email }, { anonymous: true })
}

/**
 * confirmPasswordReset calls POST /auth/password-reset/confirm, which sets
 * the new password and ends every session of the account. A wrong, expired
 * or used-up code throws `invalid_code`.
 */
export async function confirmPasswordReset(client: HttpClient, body: PasswordResetConfirmRequest): Promise<void> {
  await client.post<unknown>('/auth/password-reset/confirm', body, { anonymous: true })
}

/**
 * previewInvitation calls POST /auth/invitations/preview with the token from
 * the link. An unknown, expired, revoked or used token throws `not_found`.
 */
export function previewInvitation(client: HttpClient, token: string): Promise<InvitationPreview> {
  return client.post<InvitationPreview>('/auth/invitations/preview', { token }, { anonymous: true })
}

/**
 * acceptInvitation calls POST /auth/invitations/accept: it joins the store
 * and returns a session there. A wrong password for an existing account
 * throws `invalid_credentials`; a member already in the store, `already_member`.
 */
export function acceptInvitation(client: HttpClient, body: AcceptInvitationRequest): Promise<AuthSession> {
  return client.post<AuthSession>('/auth/invitations/accept', body, { anonymous: true })
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
