import { delay, http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from 'vitest'

import { getReadiness } from './health'
import { ApiError, createHttpClient, type SessionHooks } from './http'

const base = 'http://api.test'
const server = setupServer()

beforeAll(() => server.listen({ onUnhandledFrame: 'error' }))
afterEach(() => server.resetHandlers())
afterAll(() => server.close())

const client = createHttpClient({ baseUrl: base })

describe('createHttpClient', () => {
  it('returns the parsed JSON body', async () => {
    server.use(http.get(`${base}/readyz`, () => HttpResponse.json({ status: 'ok', checks: { postgres: 'ok' } })))

    await expect(getReadiness(client)).resolves.toEqual({ status: 'ok', checks: { postgres: 'ok' } })
  })

  it('sends JSON bodies with the right headers', async () => {
    let seen: { type: string | null; body: unknown } | undefined
    server.use(
      http.post(`${base}/echo`, async ({ request }) => {
        seen = { type: request.headers.get('Content-Type'), body: await request.json() }
        return new HttpResponse(null, { status: 204 })
      }),
    )

    await expect(client.post('/echo', { qty: 2 })).resolves.toBeUndefined()
    expect(seen).toEqual({ type: 'application/json', body: { qty: 2 } })
  })

  it('turns problem+json into an ApiError with code and field issues', async () => {
    server.use(
      http.post(`${base}/orders`, () =>
        HttpResponse.json(
          {
            type: 'about:blank',
            title: 'Dados inválidos',
            status: 400,
            code: 'invalid',
            errors: [{ field: 'phone', message: 'Informe um WhatsApp válido.' }],
          },
          { status: 400, headers: { 'Content-Type': 'application/problem+json' } },
        ),
      ),
    )

    const error = await client.post('/orders', {}).catch((e: unknown) => e)
    expect(error).toBeInstanceOf(ApiError)
    expect(error).toMatchObject({ status: 400, code: 'invalid', title: 'Dados inválidos' })
    expect((error as ApiError).fieldIssues).toEqual([{ field: 'phone', message: 'Informe um WhatsApp válido.' }])
  })

  it('handles non-JSON error bodies', async () => {
    server.use(http.get(`${base}/broken`, () => new HttpResponse('Bad Gateway', { status: 502 })))

    await expect(client.get('/broken')).rejects.toMatchObject({ status: 502, code: 'unknown' })
  })

  it('reports network failures as status 0', async () => {
    server.use(http.get(`${base}/readyz`, () => HttpResponse.error()))

    await expect(getReadiness(client)).rejects.toMatchObject({ status: 0, code: 'network' })
  })

  it('gives up on a call the server never answers, like a network failure', async () => {
    server.use(http.get(`${base}/slow`, () => delay('infinite')))

    await expect(createHttpClient({ baseUrl: base, timeoutMs: 20 }).get('/slow')).rejects.toMatchObject({ status: 0, code: 'network' })
  })

  it('reports a body cut off midway like a network failure', async () => {
    // A body that stalls until the timeout fails the same way: the read rejects.
    const cutOff = new ReadableStream({
      start(controller) {
        controller.enqueue(new TextEncoder().encode('{"items":'))
        controller.error(new Error('connection reset'))
      },
    })
    server.use(http.get(`${base}/cut-off`, () => new HttpResponse(cutOff, { headers: { 'Content-Type': 'application/json' } })))

    await expect(client.get('/cut-off')).rejects.toMatchObject({ status: 0, code: 'network' })
  })

  it('resolves an accepted call with no body, such as a 202', async () => {
    server.use(http.post(`${base}/accepted`, () => new HttpResponse(null, { status: 202 })))

    await expect(client.post('/accepted', {})).resolves.toBeUndefined()
  })

  it('reports an empty 200 as an unusable answer, since a 200 always has a body', async () => {
    server.use(http.post(`${base}/auth/login`, () => new HttpResponse(null, { status: 200 })))

    await expect(client.post('/auth/login', {})).rejects.toMatchObject({ name: 'ApiError', status: 0 })
  })

  it('reports a success that is not JSON, such as a web page from a server without the API', async () => {
    server.use(http.get(`${base}/page`, () => HttpResponse.html('<!doctype html><title>Encomendo</title>')))

    await expect(client.get('/page')).rejects.toMatchObject({ name: 'ApiError', status: 0 })
  })

  it.each(['api/v1/admin/orders', '//evil.example/x', '.evil.example/x', '@evil.example/x'])(
    'refuses the path %s, which could send the token to another host',
    async (path) => {
      await expect(client.get(path)).rejects.toThrow('path must start with a single /')
    },
  )

  it.each([
    ['30', 30],
    ['0', undefined],
    ['', undefined],
    ['Wed, 25 Nov 2026 10:00:00 GMT', undefined],
  ])('reads Retry-After %j as %s seconds', async (header, seconds) => {
    server.use(
      http.get(`${base}/limited`, () =>
        HttpResponse.json(
          { type: 'about:blank', title: 'Muitas tentativas', status: 429, code: 'rate_limited' },
          { status: 429, headers: { 'Content-Type': 'application/problem+json', 'Retry-After': header } },
        ),
      ),
    )

    await expect(client.get('/limited')).rejects.toMatchObject({ code: 'rate_limited', retryAfterSeconds: seconds })
  })

  it('sends no token to the public readiness probe', async () => {
    let authorization: string | null = 'unset'
    server.use(
      http.get(`${base}/readyz`, ({ request }) => {
        authorization = request.headers.get('Authorization')
        return HttpResponse.json({ status: 'ok' })
      }),
    )
    const withSession = createHttpClient({ baseUrl: base, session: { accessToken: () => 'access-1', refresh: async () => true, scope: () => 'ana@alta-rpm' } })

    await getReadiness(withSession)

    expect(authorization).toBeNull()
  })
})

describe('createHttpClient with a session', () => {
  const problem = { 'Content-Type': 'application/problem+json' }
  const unauthorized = () =>
    HttpResponse.json({ type: 'about:blank', title: 'Sessão expirada', status: 401, code: 'unauthenticated' }, { status: 401, headers: problem })

  /** A session that holds `token` and, when refreshed, moves to `renewed` (null: the session is over). */
  function fakeSession(token: string | null, renewed: string | null) {
    let current = token
    const refresh = vi.fn(async () => {
      current = renewed
      return renewed !== null
    })
    const hooks: SessionHooks = { accessToken: () => current, refresh, scope: () => 'ana@alta-rpm' }
    return { hooks, refresh }
  }

  /** An admin endpoint that only accepts the token "fresh". */
  const orders = () =>
    http.get(`${base}/api/v1/admin/orders`, ({ request }) =>
      request.headers.get('Authorization') === 'Bearer fresh' ? HttpResponse.json({ items: [] }) : unauthorized(),
    )

  it('sends the access token as a bearer token', async () => {
    server.use(orders())
    const { hooks, refresh } = fakeSession('fresh', null)

    await expect(createHttpClient({ baseUrl: base, session: hooks }).get('/api/v1/admin/orders')).resolves.toEqual({ items: [] })
    expect(refresh).not.toHaveBeenCalled()
  })

  it('sends no Authorization header when signed out', async () => {
    let authorization: string | null = 'unset'
    server.use(
      http.get(`${base}/api/v1/admin/members`, ({ request }) => {
        authorization = request.headers.get('Authorization')
        return HttpResponse.json({ items: [] })
      }),
    )

    await createHttpClient({ baseUrl: base, session: fakeSession(null, null).hooks }).get('/api/v1/admin/members')
    expect(authorization).toBeNull()
  })

  it('refreshes once on a 401 and retries with the new token', async () => {
    server.use(orders())
    const { hooks, refresh } = fakeSession('expired', 'fresh')

    await expect(createHttpClient({ baseUrl: base, session: hooks }).get('/api/v1/admin/orders')).resolves.toEqual({ items: [] })
    expect(refresh).toHaveBeenCalledTimes(1)
  })

  it('sends the same body and Idempotency-Key again on the retry', async () => {
    const attempts: { body: unknown; key: string | null }[] = []
    server.use(
      http.post(`${base}/api/v1/admin/orders`, async ({ request }) => {
        attempts.push({ body: await request.json(), key: request.headers.get('Idempotency-Key') })
        return request.headers.get('Authorization') === 'Bearer fresh' ? HttpResponse.json({ id: 'P-0231' }, { status: 201 }) : unauthorized()
      }),
    )

    const client = createHttpClient({ baseUrl: base, session: fakeSession('expired', 'fresh').hooks })

    await expect(
      client.post('/api/v1/admin/orders', { qty: 2 }, { headers: { 'Idempotency-Key': 'k-1' } }),
    ).resolves.toEqual({ id: 'P-0231' })
    expect(attempts).toEqual([
      { body: { qty: 2 }, key: 'k-1' },
      { body: { qty: 2 }, key: 'k-1' },
    ])
  })

  it('reuses a token another request already refreshed instead of refreshing again', async () => {
    let refreshed = () => {}
    const refreshDone = new Promise<void>((resolve) => (refreshed = resolve))
    let rejected = 0
    server.use(
      http.get(`${base}/api/v1/admin/orders`, async ({ request }) => {
        if (request.headers.get('Authorization') === 'Bearer fresh') return HttpResponse.json({ items: [] })
        rejected += 1
        // The second rejection arrives only after the first request refreshed.
        if (rejected === 2) await refreshDone
        return unauthorized()
      }),
    )
    let current = 'expired'
    const refresh = vi.fn(async () => {
      current = 'fresh'
      refreshed()
      return true
    })
    const client = createHttpClient({ baseUrl: base, session: { accessToken: () => current, refresh, scope: () => 'ana@alta-rpm' } })

    await expect(Promise.all([client.get('/api/v1/admin/orders'), client.get('/api/v1/admin/orders')])).resolves.toEqual([
      { items: [] },
      { items: [] },
    ])
    expect(refresh).toHaveBeenCalledTimes(1)
  })

  it('throws the 401 when the session cannot be refreshed', async () => {
    server.use(orders())
    const { hooks, refresh } = fakeSession('expired', null)

    await expect(createHttpClient({ baseUrl: base, session: hooks }).get('/api/v1/admin/orders')).rejects.toMatchObject({
      status: 401,
      code: 'unauthenticated',
    })
    expect(refresh).toHaveBeenCalledTimes(1)
  })

  it('refreshes only once when the retry is rejected too', async () => {
    server.use(orders())
    const { hooks, refresh } = fakeSession('expired', 'also-rejected')

    await expect(createHttpClient({ baseUrl: base, session: hooks }).get('/api/v1/admin/orders')).rejects.toMatchObject({ status: 401 })
    expect(refresh).toHaveBeenCalledTimes(1)
  })

  it('never sends the token or refreshes on anonymous requests', async () => {
    let authorization: string | null = 'unset'
    server.use(
      http.post(`${base}/auth/login`, ({ request }) => {
        authorization = request.headers.get('Authorization')
        return HttpResponse.json(
          { type: 'about:blank', title: 'E-mail ou senha incorretos', status: 401, code: 'invalid_credentials' },
          { status: 401, headers: problem },
        )
      }),
    )
    const { hooks, refresh } = fakeSession('fresh', 'fresh')

    await expect(
      createHttpClient({ baseUrl: base, session: hooks }).post('/auth/login', { email: 'dono@loja.com.br' }, { anonymous: true }),
    ).rejects.toMatchObject({ status: 401, code: 'invalid_credentials' })
    expect(authorization).toBeNull()
    expect(refresh).not.toHaveBeenCalled()
  })

  it('does not refresh on other errors', async () => {
    server.use(
      http.get(`${base}/api/v1/admin/finance`, () =>
        HttpResponse.json({ type: 'about:blank', title: 'Sem permissão', status: 403, code: 'forbidden' }, { status: 403, headers: problem }),
      ),
    )
    const { hooks, refresh } = fakeSession('fresh', 'fresh')

    await expect(createHttpClient({ baseUrl: base, session: hooks }).get('/api/v1/admin/finance')).rejects.toMatchObject({ status: 403 })
    expect(refresh).not.toHaveBeenCalled()
  })

  it('passes a failed refresh on to the caller', async () => {
    server.use(orders())
    const offline = new ApiError(0, { title: 'Sem conexão com o servidor' })
    const hooks: SessionHooks = { accessToken: () => 'expired', refresh: () => Promise.reject(offline), scope: () => 'ana@alta-rpm' }

    await expect(createHttpClient({ baseUrl: base, session: hooks }).get('/api/v1/admin/orders')).rejects.toBe(offline)
  })

  it('never retries a call under another session than the one it was made for', async () => {
    // A write meant for one store must not run in another after a switch, nor
    // as another user after a login elsewhere (ADR 0012).
    const sent: (string | null)[] = []
    server.use(
      http.post(`${base}/api/v1/admin/orders`, ({ request }) => {
        sent.push(request.headers.get('Authorization'))
        return unauthorized()
      }),
    )
    let token = 'access-store-a'
    let scope = 'ana@store-a'
    const hooks: SessionHooks = {
      accessToken: () => token,
      scope: () => scope,
      refresh: async () => {
        token = 'access-store-b'
        scope = 'ana@store-b'
        return true
      },
    }

    await expect(createHttpClient({ baseUrl: base, session: hooks }).post('/api/v1/admin/orders', { qty: 1 })).rejects.toMatchObject({
      status: 401,
    })
    expect(sent).toEqual(['Bearer access-store-a'])
  })
})
