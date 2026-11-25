import { logIn } from '@encomendo/api'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { createPinia, setActivePinia } from 'pinia'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it } from 'vitest'

import { altaRpm, authSession, gate, problem } from '../testing/fixtures'
import { useSessionStore } from './session'

const server = setupServer()

beforeAll(() => server.listen({ onUnhandledFrame: 'error' }))
afterEach(() => {
  server.resetHandlers()
  Reflect.deleteProperty(navigator, 'locks')
})
afterAll(() => server.close())
beforeEach(() => setActivePinia(createPinia()))

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
    expect(localStorage.length).toBe(0)
    expect(sessionStorage.length).toBe(0)
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
