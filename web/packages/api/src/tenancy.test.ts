import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, describe, expect, it } from 'vitest'

import { createHttpClient } from './http'
import { createTenant, type Tenant } from './tenancy'

const base = 'http://api.test'
const server = setupServer()

beforeAll(() => server.listen({ onUnhandledFrame: 'error' }))
afterEach(() => server.resetHandlers())
afterAll(() => server.close())

const client = createHttpClient({ baseUrl: base, session: { accessToken: () => 'access-1', refresh: async () => false, scope: () => 'ana@alta-rpm' } })

describe('createTenant', () => {
  it('creates the store with the access token and returns it', async () => {
    const tenant: Tenant = { id: '0190f3c4-5e1a-7b2c-8d3e-4f5a6b7c8d00', name: 'Alta RPM', slug: 'altarpm', createdAt: '2026-11-25T12:00:00Z' }
    let seen: { authorization: string | null; body: unknown } | undefined
    server.use(
      http.post(`${base}/api/v1/admin/tenants`, async ({ request }) => {
        seen = { authorization: request.headers.get('Authorization'), body: await request.json() }
        return HttpResponse.json(tenant, { status: 201 })
      }),
    )

    await expect(createTenant(client, { name: 'Alta RPM', slug: 'altarpm' })).resolves.toEqual(tenant)
    expect(seen).toEqual({ authorization: 'Bearer access-1', body: { name: 'Alta RPM', slug: 'altarpm' } })
  })

  it('reports an address in use as slug_taken', async () => {
    server.use(
      http.post(`${base}/api/v1/admin/tenants`, () =>
        HttpResponse.json(
          { type: 'about:blank', title: 'Endereço em uso', status: 409, code: 'slug_taken' },
          { status: 409, headers: { 'Content-Type': 'application/problem+json' } },
        ),
      ),
    )

    await expect(createTenant(client, { name: 'Alta RPM', slug: 'altarpm' })).rejects.toMatchObject({ status: 409, code: 'slug_taken' })
  })
})
