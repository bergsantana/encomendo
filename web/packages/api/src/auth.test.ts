import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from 'vitest'

import {
  acceptInvitation,
  type AuthSession,
  confirmPasswordReset,
  type InvitationPreview,
  logIn,
  logOut,
  previewInvitation,
  refreshSession,
  requestPasswordReset,
  signUp,
  switchTenant,
} from './auth'
import { ApiError, createHttpClient } from './http'

const base = 'http://api.test'
const server = setupServer()

beforeAll(() => server.listen({ onUnhandledFrame: 'error' }))
afterEach(() => server.resetHandlers())
afterAll(() => server.close())

const problem = { 'Content-Type': 'application/problem+json' }

const session: AuthSession = {
  accessToken: 'access-1',
  tokenType: 'Bearer',
  expiresIn: 900,
  user: { id: '0190f3c4-5e1a-7b2c-8d3e-4f5a6b7c8d9e', name: 'Ana Souza', email: 'ana@altarpm.com.br' },
  tenant: { id: '0190f3c4-5e1a-7b2c-8d3e-4f5a6b7c8d00', name: 'Alta RPM', slug: 'altarpm' },
  role: 'owner',
  permissions: ['order:read', 'order:write'],
  memberships: [{ tenant: { id: '0190f3c4-5e1a-7b2c-8d3e-4f5a6b7c8d00', name: 'Alta RPM', slug: 'altarpm' }, role: 'owner' }],
}

/** A client with a live session, to prove the auth calls never use it. */
function signedInClient() {
  const refresh = vi.fn(async () => true)
  const client = createHttpClient({ baseUrl: base, session: { accessToken: () => 'access-0', refresh, scope: () => 'ana@alta-rpm' } })
  return { client, refresh }
}

interface Seen {
  authorization: string | null
  contentType: string | null
  body: unknown
}

async function seen(request: Request): Promise<Seen> {
  return {
    authorization: request.headers.get('Authorization'),
    contentType: request.headers.get('Content-Type'),
    body: await request.json(),
  }
}

describe('logIn', () => {
  it('posts the credentials without a bearer token and returns the session', async () => {
    let request: Seen | undefined
    server.use(
      http.post(`${base}/auth/login`, async (info) => {
        request = await seen(info.request)
        return HttpResponse.json(session)
      }),
    )
    const { client } = signedInClient()

    await expect(logIn(client, { email: 'ana@altarpm.com.br', password: 'segredo-forte' })).resolves.toEqual(session)
    expect(request).toEqual({
      authorization: null,
      contentType: 'application/json',
      body: { email: 'ana@altarpm.com.br', password: 'segredo-forte' },
    })
  })

  it('reports a wrong password as invalid_credentials and never refreshes', async () => {
    server.use(
      http.post(`${base}/auth/login`, () =>
        HttpResponse.json(
          { type: 'about:blank', title: 'E-mail ou senha incorretos', status: 401, code: 'invalid_credentials' },
          { status: 401, headers: problem },
        ),
      ),
    )
    const { client, refresh } = signedInClient()

    await expect(logIn(client, { email: 'ana@altarpm.com.br', password: 'errada' })).rejects.toMatchObject({
      status: 401,
      code: 'invalid_credentials',
    })
    expect(refresh).not.toHaveBeenCalled()
  })

  it('tells how long to wait when rate-limited', async () => {
    server.use(
      http.post(`${base}/auth/login`, () =>
        HttpResponse.json(
          { type: 'about:blank', title: 'Muitas tentativas', status: 429, code: 'rate_limited' },
          { status: 429, headers: { ...problem, 'Retry-After': '30' } },
        ),
      ),
    )
    const { client } = signedInClient()

    const error = await logIn(client, { email: 'ana@altarpm.com.br', password: 'x' }).catch((e: unknown) => e)

    expect(error).toBeInstanceOf(ApiError)
    expect(error).toMatchObject({ code: 'rate_limited', retryAfterSeconds: 30 })
  })
})

describe('refreshSession', () => {
  it('sends an empty JSON body and the cookie, never the access token', async () => {
    let request: Seen | undefined
    server.use(
      http.post(`${base}/auth/refresh`, async (info) => {
        request = await seen(info.request)
        return HttpResponse.json(session)
      }),
    )
    const { client } = signedInClient()

    await expect(refreshSession(client)).resolves.toEqual(session)
    // Every /auth call must carry a JSON body (ADR 0010); the cookie carries the token.
    expect(request).toEqual({ authorization: null, contentType: 'application/json', body: {} })
  })

  it('throws the refusal without refreshing again', async () => {
    server.use(
      http.post(`${base}/auth/refresh`, () =>
        HttpResponse.json(
          { type: 'about:blank', title: 'Sessão expirada', status: 401, code: 'unauthenticated' },
          { status: 401, headers: problem },
        ),
      ),
    )
    const { client, refresh } = signedInClient()

    await expect(refreshSession(client)).rejects.toMatchObject({ status: 401, code: 'unauthenticated' })
    expect(refresh).not.toHaveBeenCalled()
  })
})

describe('signUp', () => {
  it('creates the account without a bearer token and returns a session without a store', async () => {
    const storeless: AuthSession = { ...session, tenant: undefined, role: undefined, permissions: [], memberships: [] }
    let request: Seen | undefined
    server.use(
      http.post(`${base}/auth/signup`, async (info) => {
        request = await seen(info.request)
        return HttpResponse.json(storeless, { status: 201 })
      }),
    )

    await expect(signUp(signedInClient().client, { name: 'Ana Souza', email: 'ana@altarpm.com.br', password: 'segredo-forte' })).resolves.toEqual(
      storeless,
    )
    expect(request).toEqual({
      authorization: null,
      contentType: 'application/json',
      body: { name: 'Ana Souza', email: 'ana@altarpm.com.br', password: 'segredo-forte' },
    })
  })
})

describe('switchTenant', () => {
  it('asks for a session in another store with the refresh cookie, never the access token', async () => {
    let request: Seen | undefined
    server.use(
      http.post(`${base}/auth/switch-tenant`, async (info) => {
        request = await seen(info.request)
        return HttpResponse.json(session)
      }),
    )
    const { client, refresh } = signedInClient()

    await expect(switchTenant(client, '0190f3c4-5e1a-7b2c-8d3e-4f5a6b7c8d00')).resolves.toEqual(session)
    expect(request).toEqual({
      authorization: null,
      contentType: 'application/json',
      body: { tenantId: '0190f3c4-5e1a-7b2c-8d3e-4f5a6b7c8d00' },
    })
    expect(refresh).not.toHaveBeenCalled()
  })
})

describe('password reset', () => {
  it('asks for a code by email, without a bearer token, and gets no answer about the account', async () => {
    let request: Seen | undefined
    server.use(
      http.post(`${base}/auth/password-reset`, async (info) => {
        request = await seen(info.request)
        return new HttpResponse(null, { status: 202 })
      }),
    )

    await expect(requestPasswordReset(signedInClient().client, 'ana@altarpm.com.br')).resolves.toBeUndefined()
    expect(request).toEqual({ authorization: null, contentType: 'application/json', body: { email: 'ana@altarpm.com.br' } })
  })

  it('sets the new password with the code', async () => {
    let request: Seen | undefined
    server.use(
      http.post(`${base}/auth/password-reset/confirm`, async (info) => {
        request = await seen(info.request)
        return new HttpResponse(null, { status: 204 })
      }),
    )
    const body = { email: 'ana@altarpm.com.br', code: '482913', password: 'outra-senha-forte' }

    await expect(confirmPasswordReset(signedInClient().client, body)).resolves.toBeUndefined()
    expect(request).toEqual({ authorization: null, contentType: 'application/json', body })
  })

  it('reports a wrong or expired code as invalid_code', async () => {
    server.use(
      http.post(`${base}/auth/password-reset/confirm`, () =>
        HttpResponse.json({ type: 'about:blank', title: 'Código inválido', status: 400, code: 'invalid_code' }, { status: 400, headers: problem }),
      ),
    )
    const { client, refresh } = signedInClient()

    await expect(confirmPasswordReset(client, { email: 'ana@altarpm.com.br', code: '000000', password: 'outra-senha-forte' })).rejects.toMatchObject({
      code: 'invalid_code',
    })
    expect(refresh).not.toHaveBeenCalled()
  })
})

describe('invitations', () => {
  const preview: InvitationPreview = {
    tenant: { id: '0190f3c4-5e1a-7b2c-8d3e-4f5a6b7c8d00', name: 'Alta RPM', slug: 'altarpm' },
    email: 'bruno@example.com',
    role: 'staff',
    accountExists: false,
    expiresAt: '2026-12-02T12:00:00Z',
  }

  it('previews what the link offers, without a bearer token', async () => {
    let request: Seen | undefined
    server.use(
      http.post(`${base}/auth/invitations/preview`, async (info) => {
        request = await seen(info.request)
        return HttpResponse.json(preview)
      }),
    )

    await expect(previewInvitation(signedInClient().client, 'tok-123')).resolves.toEqual(preview)
    expect(request).toEqual({ authorization: null, contentType: 'application/json', body: { token: 'tok-123' } })
  })

  it('accepts and returns the session in that store', async () => {
    let request: Seen | undefined
    server.use(
      http.post(`${base}/auth/invitations/accept`, async (info) => {
        request = await seen(info.request)
        return HttpResponse.json(session)
      }),
    )

    await expect(acceptInvitation(signedInClient().client, { token: 'tok-123', name: 'Bruno Lima', password: 'segredo-forte' })).resolves.toEqual(
      session,
    )
    expect(request).toEqual({
      authorization: null,
      contentType: 'application/json',
      body: { token: 'tok-123', name: 'Bruno Lima', password: 'segredo-forte' },
    })
  })

  it('reports a wrong password for an existing account as invalid_credentials and never refreshes', async () => {
    server.use(
      http.post(`${base}/auth/invitations/accept`, () =>
        HttpResponse.json(
          { type: 'about:blank', title: 'Senha incorreta', status: 401, code: 'invalid_credentials' },
          { status: 401, headers: problem },
        ),
      ),
    )
    const { client, refresh } = signedInClient()

    await expect(acceptInvitation(client, { token: 'tok-123', password: 'errada' })).rejects.toMatchObject({ code: 'invalid_credentials' })
    expect(refresh).not.toHaveBeenCalled()
  })
})

describe('logOut', () => {
  it('sends the access token so the API revokes it at once', async () => {
    let request: Seen | undefined
    server.use(
      http.post(`${base}/auth/logout`, async (info) => {
        request = await seen(info.request)
        return new HttpResponse(null, { status: 204 })
      }),
    )

    await expect(logOut(signedInClient().client, 'access-1')).resolves.toBeUndefined()
    expect(request).toEqual({ authorization: 'Bearer access-1', contentType: 'application/json', body: {} })
  })

  it('works without an access token', async () => {
    let authorization: string | null = 'unset'
    server.use(
      http.post(`${base}/auth/logout`, ({ request }) => {
        authorization = request.headers.get('Authorization')
        return new HttpResponse(null, { status: 204 })
      }),
    )

    await logOut(signedInClient().client, null)
    expect(authorization).toBeNull()
  })
})
