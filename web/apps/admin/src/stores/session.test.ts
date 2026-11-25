import { logIn } from '@encomendo/api'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { createPinia, setActivePinia } from 'pinia'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'

import { lastStore } from '../lastStore'
import { altaRpm, authSession, gate, problem } from '../testing/fixtures'
import { useSessionStore } from './session'

const server = setupServer()

beforeAll(() => server.listen({ onUnhandledFrame: 'error' }))
afterEach(() => {
  server.resetHandlers()
  Reflect.deleteProperty(navigator, 'locks')
  localStorage.clear()
})
afterAll(() => server.close())
// Every store the tests create is disposed afterwards, closing its BroadcastChannel,
// so no announcement from one test can reach the next.
const pinias: ReturnType<typeof createPinia>[] = []
function freshPinia() {
  const pinia = createPinia()
  pinias.push(pinia)
  setActivePinia(pinia)
  return pinia
}
beforeEach(() => freshPinia())
afterEach(() => pinias.splice(0).forEach((pinia) => useSessionStore(pinia).$dispose()))

const nextTask = () => new Promise((resolve) => setTimeout(resolve, 0))
const refreshTo = (session = authSession({ accessToken: 'access-2' })) => http.post('*/auth/refresh', () => HttpResponse.json(session))
const refused = (status: number, code: string) => http.post('*/auth/refresh', () => problem(status, code, 'Recusado'))

/** A Web Lock manager like the browser's, granting the lock only when `granted` opens. */
function fakeLocks(granted: Promise<void> = Promise.resolve()) {
  const held: string[] = []
  Object.defineProperty(navigator, 'locks', {
    configurable: true,
    value: {
      request: async (name: string, task: () => Promise<unknown>) => {
        await granted
        held.push(name)
        try {
          return await task()
        } finally {
          held.pop()
        }
      },
    },
  })
  return held
}

describe('useSessionStore: the session it holds', () => {
  it('starts signed out, with no token and no permissions', () => {
    const session = useSessionStore()

    expect(session.isSignedIn).toBe(false)
    expect(session.accessToken()).toBeNull()
    expect(session.user).toBeNull()
    expect(session.can('order:read')).toBe(false)
  })

  it('holds the user, store, role and permissions of a started session', () => {
    const session = useSessionStore()

    session.start(authSession())

    expect(session.isSignedIn).toBe(true)
    expect(session.accessToken()).toBe('access-1')
    expect(session.user?.name).toBe('Ana Souza')
    expect(session.tenant?.name).toBe('Alta RPM')
    expect(session.role).toBe('owner')
    expect(session.memberships).toHaveLength(1)
    expect(session.can('order:write')).toBe(true)
    expect(session.can('finance:read')).toBe(false)
  })

  it('forgets everything when the session ends, and says why', () => {
    const session = useSessionStore()
    session.start(authSession())

    session.end('logged_out')

    expect(session.isSignedIn).toBe(false)
    expect(session.accessToken()).toBeNull()
    expect(session.tenant).toBeNull()
    expect(session.can('order:read')).toBe(false)
    expect(session.ended).toBe('logged_out')
  })

  it('keeps the access token out of the store state and out of web storage', () => {
    const session = useSessionStore()

    session.start(authSession({ accessToken: 'secret-access-token', refreshToken: 'secret-refresh-token' }))

    // Store state is what devtools show and persistence plugins write to
    // localStorage, so neither token may ever be part of it.
    const state = JSON.stringify(session.$state)
    expect(state).not.toContain('secret-access-token')
    expect(state).not.toContain('secret-refresh-token')
    const stored = JSON.stringify({ ...localStorage, ...sessionStorage })
    expect(stored).not.toContain('secret-access-token')
    expect(stored).not.toContain('secret-refresh-token')
  })

  it('remembers the store on this device, for the next login', () => {
    useSessionStore().start(authSession())

    expect(lastStore()).toBe(altaRpm.id)
  })
})

describe('useSessionStore: switchTo', () => {
  it('moves the session to another store, in its turn', async () => {
    const other = { ...altaRpm, id: '0190f3c4-5e1a-7b2c-8d3e-00000000cafe', name: 'Moto Peças Sul', slug: 'motopecas' }
    let body: unknown
    server.use(
      http.post('*/auth/switch-tenant', async ({ request }) => {
        body = await request.json()
        return HttpResponse.json(authSession({ accessToken: 'access-other', tenant: other }))
      }),
    )
    const session = useSessionStore()
    session.start(authSession())

    await session.switchTo(other.id)

    expect(body).toEqual({ tenantId: other.id })
    expect(lastStore()).toBe(other.id)
  })

  it('stops acting in either store as soon as the switch is done, until the page loads the new one', async () => {
    // Nothing meant for the old store may run with the new store's token in this page.
    server.use(
      http.post('*/auth/switch-tenant', () => HttpResponse.json(authSession({ accessToken: 'access-other' }))),
      refreshTo(),
    )
    const session = useSessionStore()
    session.start(authSession())

    await session.switchTo(altaRpm.id)

    expect(session.ended).toBe('switched')
    expect(session.accessToken()).toBeNull()
    expect(session.scope()).toBeNull()
    await expect(session.refresh()).resolves.toBe(false)
  })

  it('refreshes once after a 403, so a store the member lost leaves the list', async () => {
    server.use(
      http.post('*/auth/switch-tenant', () => problem(403, 'forbidden', 'Sem acesso')),
      refreshTo(authSession({ memberships: [] })),
    )
    const session = useSessionStore()
    session.start(authSession())

    await expect(session.switchTo(altaRpm.id)).rejects.toMatchObject({ status: 403 })

    expect(session.memberships).toEqual([])
    expect(session.ended).toBeNull()
  })

  it('refreshes once after a 401 and ends the session when that is refused too', async () => {
    server.use(
      http.post('*/auth/switch-tenant', () => problem(401, 'unauthenticated', 'Sessão expirada')),
      refused(401, 'unauthenticated'),
    )
    const session = useSessionStore()
    session.start(authSession())

    await expect(session.switchTo(altaRpm.id)).rejects.toMatchObject({ status: 401 })

    expect(session.ended).toBe('expired')
  })
})

describe('useSessionStore: scope', () => {
  it('names the user and the store the session acts for', () => {
    const session = useSessionStore()

    session.start(authSession())

    expect(session.scope()).toBe(`${authSession().user.id}@${altaRpm.id}`)
  })

  it('is empty when signed out', () => {
    expect(useSessionStore().scope()).toBeNull()
  })
})

describe('useSessionStore: other tabs', () => {
  const bruno = authSession({
    accessToken: 'access-bruno',
    user: { id: '0190f3c4-5e1a-7b2c-8d3e-00000000beef', name: 'Bruno Lima', email: 'bruno@example.com' },
  })

  /** tab opens the app in another tab: its own Pinia, the same BroadcastChannel. */
  function tab() {
    freshPinia()
    return useSessionStore()
  }

  /**
   * delivered resolves once the test's own listener heard the announcement,
   * plus a little slack for the other tabs. Only the test that checks that
   * nothing happens needs it; the others wait for the state they expect.
   */
  async function delivered() {
    await vi.waitFor(() => expect(heard.length).toBeGreaterThan(0))
    await new Promise((resolve) => setTimeout(resolve, 10))
  }

  let listener: BroadcastChannel
  let heard: unknown[]
  beforeEach(() => {
    heard = []
    listener = new BroadcastChannel('encomendo-session')
    listener.onmessage = (event: MessageEvent) => heard.push(event.data)
  })
  afterEach(() => listener.close())

  it('ends here, as replaced, when another tab logs in as someone else', async () => {
    server.use(http.post('*/auth/login', () => HttpResponse.json(bruno)))
    const here = tab()
    here.start(authSession())

    await tab().begin((client) => logIn(client, { email: 'bruno@example.com', password: 'segredo-forte' }))
    await vi.waitFor(() => expect(here.ended).toBe('replaced'))
  })

  it('ends here, as replaced, when another tab switches store', async () => {
    const other = { ...altaRpm, id: '0190f3c4-5e1a-7b2c-8d3e-00000000cafe' }
    server.use(http.post('*/auth/switch-tenant', () => HttpResponse.json(authSession({ tenant: other }))))
    const here = tab()
    here.start(authSession())
    const there = tab()
    there.start(authSession())

    await there.switchTo(other.id)
    await vi.waitFor(() => expect(here.ended).toBe('replaced'))
  })

  it('ends here, as logged out, when another tab logs out', async () => {
    server.use(http.post('*/auth/logout', () => new HttpResponse(null, { status: 204 })))
    const here = tab()
    here.start(authSession())
    const there = tab()
    there.start(authSession())

    await there.logOut()
    await vi.waitFor(() => expect(here.ended).toBe('logged_out'))
  })

  it('carries on when another tab logs in as the same member in the same store', async () => {
    server.use(http.post('*/auth/login', () => HttpResponse.json(authSession({ accessToken: 'access-there' }))))
    const here = tab()
    here.start(authSession())

    await tab().begin((client) => logIn(client, { email: 'ana@altarpm.com.br', password: 'segredo-forte' }))
    await delivered()

    expect(here.ended).toBeNull()
    expect(here.isSignedIn).toBe(true)
  })

  it('tells other tabs only whose session it is, never a token', async () => {
    server.use(http.post('*/auth/login', () => HttpResponse.json(bruno)))

    await tab().begin((client) => logIn(client, { email: 'bruno@example.com', password: 'segredo-forte' }))
    await vi.waitFor(() => expect(heard).toEqual([{ userId: bruno.user.id, tenantId: altaRpm.id }]))
  })
})

describe('useSessionStore: refresh', () => {
  it('trades the cookie for a new session', async () => {
    server.use(refreshTo(authSession({ accessToken: 'access-2', permissions: ['order:read'] })))
    const session = useSessionStore()
    session.start(authSession())

    await expect(session.refresh()).resolves.toBe(true)

    expect(session.accessToken()).toBe('access-2')
    expect(session.can('order:write')).toBe(false)
  })

  it.each([
    [400, 'invalid'],
    [401, 'unauthenticated'],
    [403, 'forbidden'],
  ])('ends the session as expired when the refresh is refused with %i', async (status, code) => {
    server.use(refused(status, code))
    const session = useSessionStore()
    session.start(authSession())

    await expect(session.refresh()).resolves.toBe(false)

    expect(session.isSignedIn).toBe(false)
    expect(session.ended).toBe('expired')
  })

  it.each([
    ['offline', () => HttpResponse.error()],
    ['rate-limited', () => problem(429, 'rate_limited', 'Muitas tentativas', { 'Retry-After': '30' })],
    ['down', () => problem(503, 'not_ready', 'Serviço indisponível')],
  ])('keeps the session when the server is %s, and passes the error on', async (_, answer) => {
    server.use(http.post('*/auth/refresh', answer))
    const session = useSessionStore()
    session.start(authSession())

    await expect(session.refresh()).rejects.toMatchObject({ name: 'ApiError' })

    expect(session.accessToken()).toBe('access-1')
    expect(session.ended).toBeNull()
  })

  it.each([
    ['another user', authSession({ user: { id: '0190f3c4-5e1a-7b2c-8d3e-00000000beef', name: 'Bruno Lima', email: 'bruno@example.com' } })],
    ['another store', authSession({ tenant: { ...altaRpm, id: '0190f3c4-5e1a-7b2c-8d3e-00000000cafe', name: 'Moto Peças Sul' } })],
  ])('ends the session instead of turning into %s, when the cookie changed hands', async (_, theirs) => {
    // Another tab logged in or switched store: the shared cookie is theirs now.
    server.use(refreshTo(theirs))
    const session = useSessionStore()
    session.start(authSession())

    await expect(session.refresh()).resolves.toBe(false)

    expect(session.isSignedIn).toBe(false)
    expect(session.ended).toBe('replaced')
  })

  it('shares one refresh between callers, so the cookie is presented once', async () => {
    const answer = gate()
    let calls = 0
    server.use(
      http.post('*/auth/refresh', async () => {
        calls += 1
        await answer.opened
        return HttpResponse.json(authSession({ accessToken: 'access-2' }))
      }),
    )
    const session = useSessionStore()

    const both = Promise.all([session.refresh(), session.refresh()])
    answer.open()

    await expect(both).resolves.toEqual([true, true])
    expect(calls).toBe(1)
  })

  it('lets a logout made during the refresh win', async () => {
    const answer = gate()
    server.use(
      http.post('*/auth/refresh', async () => {
        await answer.opened
        return HttpResponse.json(authSession({ accessToken: 'access-2' }))
      }),
    )
    const session = useSessionStore()
    session.start(authSession())

    const refreshing = session.refresh()
    session.end('logged_out')
    answer.open()

    await expect(refreshing).resolves.toBe(false)
    expect(session.isSignedIn).toBe(false)
  })

  it('lets a logout made while the refresh waits for the lock win', async () => {
    const lock = gate()
    fakeLocks(lock.opened)
    server.use(refreshTo())
    const session = useSessionStore()
    session.start(authSession())

    const refreshing = session.refresh()
    session.end('logged_out')
    lock.open()

    await expect(refreshing).resolves.toBe(false)
    expect(session.isSignedIn).toBe(false)
  })

  it('does not send a refresh still waiting its turn when a logout lands first', async () => {
    // The cookie in the jar may already be someone else's by then.
    const lock = gate()
    fakeLocks(lock.opened)
    let calls = 0
    server.use(
      http.post('*/auth/refresh', () => {
        calls += 1
        return HttpResponse.json(authSession())
      }),
    )
    const session = useSessionStore()
    session.start(authSession())

    const refreshing = session.refresh()
    session.end('logged_out')
    lock.open()
    await refreshing

    expect(calls).toBe(0)
  })

  it('lets a login made during the refresh win', async () => {
    const answer = gate()
    server.use(
      http.post('*/auth/refresh', async () => {
        await answer.opened
        return problem(401, 'unauthenticated', 'Sessão expirada')
      }),
    )
    const session = useSessionStore()

    const refreshing = session.refresh()
    session.start(authSession({ accessToken: 'access-from-login' }))
    answer.open()

    await expect(refreshing).resolves.toBe(true)
    expect(session.accessToken()).toBe('access-from-login')
  })

  it('holds the encomendo-refresh Web Lock while it refreshes, so tabs take turns', async () => {
    const held = fakeLocks()
    let heldDuringRequest = false
    server.use(
      http.post('*/auth/refresh', () => {
        heldDuringRequest = held.includes('encomendo-refresh')
        return HttpResponse.json(authSession())
      }),
    )

    await useSessionStore().refresh()

    expect(heldDuringRequest).toBe(true)
  })
})

describe('useSessionStore: calls that set the refresh cookie wait their turn', () => {
  /** A refresh in flight, and the order in which the server saw the calls. */
  function refreshInFlight() {
    const answer = gate()
    const seen: string[] = []
    server.use(
      http.post('*/auth/refresh', async () => {
        seen.push('refresh')
        await answer.opened
        return HttpResponse.json(authSession({ accessToken: 'access-2' }))
      }),
    )
    return { answer, seen }
  }

  it('logs in only after a refresh in flight, so the refresh cannot overwrite the new cookie', async () => {
    const { answer, seen } = refreshInFlight()
    server.use(
      http.post('*/auth/login', () => {
        seen.push('login')
        return HttpResponse.json(authSession({ accessToken: 'access-from-login' }))
      }),
    )
    const session = useSessionStore()

    const refreshing = session.refresh()
    const loggingIn = session.begin((client) => logIn(client, { email: 'ana@altarpm.com.br', password: 'segredo-forte' }))
    answer.open()
    await Promise.all([refreshing, loggingIn])

    expect(seen).toEqual(['refresh', 'login'])
    expect(session.accessToken()).toBe('access-from-login')
  })

  it('logs out after a refresh in flight, presenting the newest tokens', async () => {
    const { answer, seen } = refreshInFlight()
    let authorization: string | null = null
    server.use(
      http.post('*/auth/logout', ({ request }) => {
        seen.push('logout')
        authorization = request.headers.get('Authorization')
        return new HttpResponse(null, { status: 204 })
      }),
    )
    const session = useSessionStore()
    session.start(authSession())

    const refreshing = session.refresh()
    const loggingOut = session.logOut()
    answer.open()
    await Promise.all([refreshing, loggingOut])

    expect(seen).toEqual(['refresh', 'logout'])
    expect(authorization).toBe('Bearer access-2')
    expect(session.ended).toBe('logged_out')
  })

  it('also takes turns through the Web Lock', async () => {
    const held = fakeLocks()
    let heldDuringLogin = false
    server.use(
      http.post('*/auth/login', () => {
        heldDuringLogin = held.includes('encomendo-refresh')
        return HttpResponse.json(authSession())
      }),
    )

    await useSessionStore().begin((client) => logIn(client, { email: 'ana@altarpm.com.br', password: 'segredo-forte' }))

    expect(heldDuringLogin).toBe(true)
  })
})

describe('useSessionStore: restore', () => {
  it('restores the session from the cookie once per page load', async () => {
    let calls = 0
    server.use(
      http.post('*/auth/refresh', () => {
        calls += 1
        return HttpResponse.json(authSession())
      }),
    )
    const session = useSessionStore()

    await session.restore()
    await session.restore()

    expect(session.user?.name).toBe('Ana Souza')
    expect(calls).toBe(1)
  })

  it('stays signed out when there is no session, without asking again', async () => {
    let calls = 0
    server.use(
      http.post('*/auth/refresh', () => {
        calls += 1
        return problem(401, 'unauthenticated', 'Sessão expirada')
      }),
    )
    const session = useSessionStore()

    await expect(session.restore()).resolves.toBeUndefined()
    await session.restore()

    expect(session.isSignedIn).toBe(false)
    expect(session.restoreError).toBeNull()
    expect(session.ended).toBeNull()
    expect(calls).toBe(1)
  })

  it.each([
    ['is down', () => problem(503, 'not_ready', 'Serviço indisponível'), 503],
    ['is unreachable', () => HttpResponse.error(), 0],
    // A static server without the API answers a POST with the HTML page.
    ['answers with a web page', () => HttpResponse.html('<!doctype html><title>Encomendo</title>'), 0],
  ])('stays signed out when the server %s, says why, and tries again next time', async (_, answer, status) => {
    let calls = 0
    server.use(
      http.post('*/auth/refresh', () => {
        calls += 1
        return answer()
      }),
    )
    const session = useSessionStore()

    await expect(session.restore()).resolves.toBeUndefined()

    expect(session.isSignedIn).toBe(false)
    expect(session.restoreError?.status).toBe(status)
    await session.restore()
    expect(calls).toBe(1) // the same navigation shares the failure
    await nextTask()
    await session.restore()
    expect(calls).toBe(2) // a later one asks again
  })

  it('does not ask the server once signed in', async () => {
    let calls = 0
    server.use(
      http.post('*/auth/refresh', () => {
        calls += 1
        return HttpResponse.json(authSession())
      }),
    )
    const session = useSessionStore()
    session.start(authSession())

    await session.restore()

    expect(calls).toBe(0)
  })

  it('forgets why the restore failed once a session starts', async () => {
    server.use(http.post('*/auth/refresh', () => HttpResponse.error()))
    const session = useSessionStore()
    await session.restore()

    session.start(authSession())

    expect(session.restoreError).toBeNull()
  })
})

describe('useSessionStore: logOut', () => {
  it('revokes the session on the server with its access token, then forgets it', async () => {
    let authorization: string | null = null
    server.use(
      http.post('*/auth/logout', ({ request }) => {
        authorization = request.headers.get('Authorization')
        return new HttpResponse(null, { status: 204 })
      }),
    )
    const session = useSessionStore()
    session.start(authSession())

    await session.logOut()

    expect(authorization).toBe('Bearer access-1')
    expect(session.isSignedIn).toBe(false)
    expect(session.ended).toBe('logged_out')
  })

  it('stays signed in when the logout cannot reach the server', async () => {
    // The refresh cookie is httpOnly, so only the server can end it. Saying
    // "signed out" here would let the next person reload into this session.
    server.use(http.post('*/auth/logout', () => HttpResponse.error()))
    const session = useSessionStore()
    session.start(authSession())

    await expect(session.logOut()).rejects.toMatchObject({ code: 'network' })

    expect(session.isSignedIn).toBe(true)
  })
})
