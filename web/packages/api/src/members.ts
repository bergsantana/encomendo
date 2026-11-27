import type { HttpClient } from './http'
import type { PageRequest } from './query'
import type { components } from './types/schema'

type Schemas = components['schemas']

/** Member is one person on the store's team ("Equipe"), with the role there. */
export type Member = Schemas['Member']
/** MemberPage is one page of the team, oldest member first. */
export type MemberPage = Schemas['MemberPage']
/** Invitation is an invitation not yet accepted, revoked or expired. */
export type Invitation = Schemas['Invitation']
/** InvitationPage is one page of pending invitations, newest first. */
export type InvitationPage = Schemas['InvitationPage']
/** InvitableRole is any role but owner: a store has one owner. */
export type InvitableRole = Schemas['InvitableRole']
/** InviteMemberRequest is the body of POST /api/v1/admin/invitations. */
export type InviteMemberRequest = Schemas['InviteMemberRequest']

export type { PageRequest }

// Every call here needs `members:manage`; the API re-reads the caller's role
// on each one, so a demoted admin loses them at once (ADR 0011).

/** listMembers calls GET /api/v1/admin/members. */
export function listMembers(client: HttpClient, page: PageRequest = {}): Promise<MemberPage> {
  return client.get<MemberPage>(`/api/v1/admin/members${pageQuery(page)}`)
}

/**
 * changeMemberRole calls PUT /api/v1/admin/members/{userId}/role. The owner's
 * role never changes (`forbidden`); the new role applies at the member's next
 * refresh, at most 15 minutes later.
 */
export function changeMemberRole(client: HttpClient, userId: string, role: InvitableRole): Promise<Member> {
  return client.put<Member>(`/api/v1/admin/members/${encodeURIComponent(userId)}/role`, { role })
}

/** removeMember calls DELETE /api/v1/admin/members/{userId}. The owner can never be removed (`forbidden`). */
export async function removeMember(client: HttpClient, userId: string): Promise<void> {
  await client.delete<unknown>(`/api/v1/admin/members/${encodeURIComponent(userId)}`)
}

/** listInvitations calls GET /api/v1/admin/invitations. */
export function listInvitations(client: HttpClient, page: PageRequest = {}): Promise<InvitationPage> {
  return client.get<InvitationPage>(`/api/v1/admin/invitations${pageQuery(page)}`)
}

/**
 * inviteMember calls POST /api/v1/admin/invitations, which emails a link. An
 * email with a pending invitation gets a new one in its place, which is how
 * "Reenviar" works. Someone already on the team throws `already_member`; an
 * email that could not be sent throws `email_unavailable`, with the
 * invitation saved.
 */
export function inviteMember(client: HttpClient, body: InviteMemberRequest): Promise<Invitation> {
  return client.post<Invitation>('/api/v1/admin/invitations', body)
}

/** revokeInvitation calls DELETE /api/v1/admin/invitations/{invitationId}; its link stops working. */
export async function revokeInvitation(client: HttpClient, invitationId: string): Promise<void> {
  await client.delete<unknown>(`/api/v1/admin/invitations/${encodeURIComponent(invitationId)}`)
}

function pageQuery(page: PageRequest): string {
  const query = new URLSearchParams()
  if (page.cursor !== undefined) query.set('cursor', page.cursor)
  if (page.limit !== undefined) query.set('limit', String(page.limit))
  const text = query.toString()
  return text === '' ? '' : `?${text}`
}
