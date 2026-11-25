import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, describe, expect, it } from 'vitest'

import { createHttpClient } from './http'
import {
  changeMemberRole,
  type Invitation,
  inviteMember,
  listInvitations,
  listMembers,
  type Member,
  removeMember,
  revokeInvitation,
} from './members'

const base = 'http://api.test'
const server = setupServer()

beforeAll(() => server.listen({ onUnhandledFrame: 'error' }))
afterEach(() => server.resetHandlers())
afterAll(() => server.close())

const client = createHttpClient({ baseUrl: base, session: { accessToken: () => 'access-1', refresh: async () => false, scope: () => 'ana@alta' } })

const bruno: Member = {
  userId: '0190f3c4-5e1a-7b2c-8d3e-00000000beef',
  name: 'Bruno Lima',
  email: 'bruno@example.com',
  role: 'staff',
  joinedAt: '2026-11-20T12:00:00Z',
}
const invitation: Invitation = {
  id: '0190f3c4-5e1a-7b2c-8d3e-00000000a11a',
  email: 'carla@example.com',
  role: 'viewer',
  invitedAt: '2026-11-25T12:00:00Z',
  expiresAt: '2026-12-02T12:00:00Z',
}

describe('members', () => {
  it('lists a page of the team with the access token', async () => {
    let seen: { authorization: string | null; query: string } | undefined
    server.use(
      http.get(`${base}/api/v1/admin/members`, ({ request }) => {
        seen = { authorization: request.headers.get('Authorization'), query: new URL(request.url).search }
        return HttpResponse.json({ items: [bruno], nextCursor: 'next-1' })
      }),
    )

    await expect(listMembers(client, { cursor: 'abc', limit: 50 })).resolves.toEqual({ items: [bruno], nextCursor: 'next-1' })
    expect(seen).toEqual({ authorization: 'Bearer access-1', query: '?cursor=abc&limit=50' })
  })

  it('asks for the first page without a cursor', async () => {
    let query: string | undefined
    server.use(
      http.get(`${base}/api/v1/admin/members`, ({ request }) => {
        query = new URL(request.url).search
        return HttpResponse.json({ items: [] })
      }),
    )

    await listMembers(client)
    expect(query).toBe('')
  })

  it('changes a role and returns the member', async () => {
    let body: unknown
    server.use(
      http.put(`${base}/api/v1/admin/members/${bruno.userId}/role`, async ({ request }) => {
        body = await request.json()
        return HttpResponse.json({ ...bruno, role: 'admin' })
      }),
    )

    await expect(changeMemberRole(client, bruno.userId, 'admin')).resolves.toMatchObject({ role: 'admin' })
    expect(body).toEqual({ role: 'admin' })
  })

  it('removes someone from the team', async () => {
    let removed = false
    server.use(
      http.delete(`${base}/api/v1/admin/members/${bruno.userId}`, () => {
        removed = true
        return new HttpResponse(null, { status: 204 })
      }),
    )

    await expect(removeMember(client, bruno.userId)).resolves.toBeUndefined()
    expect(removed).toBe(true)
  })

  it('keeps an id from changing the path', async () => {
    let path: string | undefined
    server.use(
      http.delete(`${base}/api/v1/admin/members/*`, ({ request }) => {
        path = new URL(request.url).pathname
        return new HttpResponse(null, { status: 204 })
      }),
    )

    await removeMember(client, '../tenants/x')
    expect(path).toBe('/api/v1/admin/members/..%2Ftenants%2Fx')
  })
})

describe('invitations', () => {
  it('lists the pending invitations', async () => {
    server.use(http.get(`${base}/api/v1/admin/invitations`, () => HttpResponse.json({ items: [invitation] })))

    await expect(listInvitations(client)).resolves.toEqual({ items: [invitation] })
  })

  it('invites an email with a role', async () => {
    let body: unknown
    server.use(
      http.post(`${base}/api/v1/admin/invitations`, async ({ request }) => {
        body = await request.json()
        return HttpResponse.json(invitation, { status: 201 })
      }),
    )

    await expect(inviteMember(client, { email: 'carla@example.com', role: 'viewer' })).resolves.toEqual(invitation)
    expect(body).toEqual({ email: 'carla@example.com', role: 'viewer' })
  })

  it('cancels an invitation', async () => {
    let revoked = false
    server.use(
      http.delete(`${base}/api/v1/admin/invitations/${invitation.id}`, () => {
        revoked = true
        return new HttpResponse(null, { status: 204 })
      }),
    )

    await revokeInvitation(client, invitation.id)
    expect(revoked).toBe(true)
  })
})
