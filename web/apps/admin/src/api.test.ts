import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { createPinia, setActivePinia } from 'pinia'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from './api'
import { useSessionStore } from './stores/session'
import { authSession, gate, problem } from './testing/fixtures'

const server = setupServer()
beforeAll(() => server.listen({ onUnhandledFrame: 'error' }))
afterEach(() => server.resetHandlers())
afterAll(() => server.close())
beforeEach(() => setActivePinia(createPinia()))

/** An admin route; it accepts only the refreshed token. */
const acceptsOnly = (token: string) =>
  http.get('*/api/v1/admin/members', ({ request }) =>
    request.headers.get('Authorization') === `Bearer ${token}`
      ? HttpResponse.json({ items: [] })
      : problem(401, 'unauthenticated', 'Sessão expirada'),
  )

describe('api', () => {
  it("sends the session's token and refreshes it once when it has expired", async () => {
    server.use(
      acceptsOnly('access-2'),
      http.post('*/auth/refresh', () => HttpResponse.json(authSession({ accessToken: 'access-2' }))),
    )
    const session = useSessionStore()
    session.start(authSession({ accessToken: 'access-1' }))

    await expect(api.get('/api/v1/admin/members')).resolves.toEqual({ items: [] })
    expect(session.accessToken()).toBe('access-2')
  })

  it('ends the session when the refresh is refused', async () => {
    server.use(
      acceptsOnly('access-2'),
      http.post('*/auth/refresh', () => problem(401, 'unauthenticated', 'Sessão expirada')),
    )
    const session = useSessionStore()
    session.start(authSession({ accessToken: 'access-1' }))

    await expect(api.get('/api/v1/admin/members')).rejects.toMatchObject({ status: 401 })
    expect(session.isSignedIn).toBe(false)
  })

  it("never retries a call under another tab's session once this one ended", async () => {
    const bruno = authSession({
      accessToken: 'access-bruno',
      user: { id: '0190f3c4-5e1a-7b2c-8d3e-00000000beef', name: 'Bruno Lima', email: 'bruno@example.com' },
    })
    let refreshes = 0
    let retriedWith: string | null = null
    server.use(
      http.post('*/auth/refresh', () => {
        refreshes += 1
        return HttpResponse.json(bruno)
      }),
      http.post('*/api/v1/admin/orders', ({ request }) => {
        retriedWith = request.headers.get('Authorization')
        return problem(401, 'unauthenticated', 'Sessão expirada')
      }),
    )
    const session = useSessionStore()
    session.start(authSession({ accessToken: 'access-ana' }))
    await session.refresh() // the cookie is Bruno's now: Ana's session ends as replaced

    // A write that answers 401 after the end, e.g. one sent just before it.
    await expect(api.post('/api/v1/admin/orders', { qty: 1 })).rejects.toMatchObject({ status: 401 })

    expect(refreshes).toBe(1)
    expect(retriedWith).toBeNull()
    expect(session.user).toBeNull()
    expect(session.ended).toBe('replaced')
  })

  it("never runs a write meant for one store with the next store's token", async () => {
    const answer = gate()
    const sentWith: (string | null)[] = []
    server.use(
      http.post('*/api/v1/admin/orders', async ({ request }) => {
        sentWith.push(request.headers.get('Authorization'))
        await answer.opened
        return problem(401, 'unauthenticated', 'Sessão expirada')
      }),
      http.post('*/auth/switch-tenant', () => HttpResponse.json(authSession({ accessToken: 'access-store-b' }))),
    )
    const session = useSessionStore()
    session.start(authSession({ accessToken: 'access-store-a' }))

    const write = api.post('/api/v1/admin/orders', { qty: 1 })
    await vi.waitFor(() => expect(sentWith).toHaveLength(1))
    await session.switchTo('0190f3c4-5e1a-7b2c-8d3e-00000000cafe')
    answer.open()

    await expect(write).rejects.toMatchObject({ status: 401 })
    expect(sentWith).toEqual(['Bearer access-store-a'])
  })

  it('does not retry a write whose 401 arrives while the switch is still on its way', async () => {
    const switching = gate()
    const sentWith: (string | null)[] = []
    server.use(
      http.post('*/api/v1/admin/orders', ({ request }) => {
        sentWith.push(request.headers.get('Authorization'))
        return problem(401, 'unauthenticated', 'Sessão expirada')
      }),
      http.post('*/auth/switch-tenant', async () => {
        await switching.opened
        return HttpResponse.json(authSession({ accessToken: 'access-store-b' }))
      }),
      http.post('*/auth/refresh', () => HttpResponse.json(authSession({ accessToken: 'access-store-b' }))),
    )
    const session = useSessionStore()
    session.start(authSession({ accessToken: 'access-store-a' }))

    const switched = session.switchTo('0190f3c4-5e1a-7b2c-8d3e-00000000cafe')
    // The write's refresh queues behind the switch, which holds the turn.
    const write = api.post('/api/v1/admin/orders', { qty: 1 })
    await vi.waitFor(() => expect(sentWith).toHaveLength(1))
    switching.open()

    await switched
    await expect(write).rejects.toMatchObject({ status: 401 })
    expect(sentWith).toEqual(['Bearer access-store-a'])
  })
})
