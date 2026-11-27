import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, describe, expect, it } from 'vitest'

import { ApiError, createHttpClient } from './http'
import { createSupplier, getSupplier, listSuppliers, replaceSupplier } from './suppliers'

const base = 'http://api.test'
const server = setupServer()

beforeAll(() => server.listen({ onUnhandledFrame: 'error' }))
afterEach(() => server.resetHandlers())
afterAll(() => server.close())

const client = createHttpClient({ baseUrl: base, session: { accessToken: () => 'access-1', refresh: async () => false, scope: () => 'ana@alta' } })

const supplierId = '0190f3c4-5e1a-7b2c-8d3e-0000000000f1'
const supplier = { id: supplierId, name: 'Moto Peças', whatsappPhone: '5511999990000', version: 1 }

function problem(status: number, code: string): Response {
  return HttpResponse.json({ status, code, title: 'Problema' }, { status, headers: { 'Content-Type': 'application/problem+json' } })
}

describe('suppliers', () => {
  it('lists a page with cursor, limit and search', async () => {
    let seen: { authorization: string | null; query: Record<string, string> } | undefined
    server.use(
      http.get(`${base}/api/v1/admin/suppliers`, ({ request }) => {
        seen = {
          authorization: request.headers.get('Authorization'),
          query: Object.fromEntries(new URL(request.url).searchParams),
        }
        return HttpResponse.json({ items: [supplier], nextCursor: 'n1' })
      }),
    )

    await expect(listSuppliers(client, { cursor: 'c1', limit: 30, q: 'moto' })).resolves.toEqual({ items: [supplier], nextCursor: 'n1' })
    expect(seen).toEqual({ authorization: 'Bearer access-1', query: { cursor: 'c1', limit: '30', q: 'moto' } })
  })

  it('asks for the first page without a query', async () => {
    let search: string | undefined
    server.use(
      http.get(`${base}/api/v1/admin/suppliers`, ({ request }) => {
        search = new URL(request.url).search
        return HttpResponse.json({ items: [] })
      }),
    )
    await listSuppliers(client)
    expect(search).toBe('')
  })

  it('creates a supplier', async () => {
    let body: unknown
    server.use(
      http.post(`${base}/api/v1/admin/suppliers`, async ({ request }) => {
        body = await request.json()
        return HttpResponse.json(supplier, { status: 201 })
      }),
    )
    await expect(createSupplier(client, { name: 'Moto Peças', whatsappPhone: '5511999990000' })).resolves.toEqual(supplier)
    expect(body).toEqual({ name: 'Moto Peças', whatsappPhone: '5511999990000' })
  })

  it('gets one supplier', async () => {
    server.use(http.get(`${base}/api/v1/admin/suppliers/${supplierId}`, () => HttpResponse.json(supplier)))
    await expect(getSupplier(client, supplierId)).resolves.toEqual(supplier)
  })

  it('replaces a supplier with the version read', async () => {
    let body: unknown
    server.use(
      http.put(`${base}/api/v1/admin/suppliers/${supplierId}`, async ({ request }) => {
        body = await request.json()
        return HttpResponse.json({ ...supplier, version: 2 })
      }),
    )
    await expect(replaceSupplier(client, supplierId, { name: 'Moto Peças SP', whatsappPhone: '5511999990000', version: 1 })).resolves.toMatchObject({
      version: 2,
    })
    expect(body).toEqual({ name: 'Moto Peças SP', whatsappPhone: '5511999990000', version: 1 })
  })

  it('surfaces version_conflict and not_found', async () => {
    server.use(
      http.put(`${base}/api/v1/admin/suppliers/${supplierId}`, () => problem(409, 'version_conflict')),
      http.get(`${base}/api/v1/admin/suppliers/${supplierId}`, () => problem(404, 'not_found')),
    )

    const conflict = await replaceSupplier(client, supplierId, { name: 'x', whatsappPhone: '1', version: 1 }).catch((error: unknown) => error)
    const missing = await getSupplier(client, supplierId).catch((error: unknown) => error)
    expect(conflict).toBeInstanceOf(ApiError)
    expect(conflict).toMatchObject({ status: 409, code: 'version_conflict' })
    expect(missing).toMatchObject({ status: 404, code: 'not_found' })
  })
})
