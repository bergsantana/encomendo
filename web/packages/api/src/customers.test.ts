import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, describe, expect, it } from 'vitest'

import { createCustomer, getCustomer, listCustomers, updateCustomer } from './customers'
import { ApiError, createHttpClient } from './http'

/** at reads a recorded call, failing the test with a clear message when it never happened. */
function at<T>(list: readonly T[], index = 0): T {
  const found = list[index]
  if (found === undefined) throw new Error(`call ${index} never happened`)
  return found
}

const base = 'http://api.test'
const server = setupServer()

beforeAll(() => server.listen({ onUnhandledFrame: 'error' }))
afterEach(() => server.resetHandlers())
afterAll(() => server.close())

const client = createHttpClient({ baseUrl: base, session: { accessToken: () => 'access-1', refresh: async () => false, scope: () => 'ana@alta' } })

const customerId = '0190f3c4-5e1a-7b2c-8d3e-0000000000c1'
const customer = { id: customerId, name: 'João Pereira', phone: '5592991234567', email: null, notes: '', addresses: [], version: 1 }

function problem(status: number, code: string): Response {
  return HttpResponse.json({ status, code, title: 'Problema' }, { status, headers: { 'Content-Type': 'application/problem+json' } })
}

describe('customers', () => {
  it('lists a page with cursor, limit and search, and leaves out what is not set', async () => {
    const seen: URL[] = []
    server.use(
      http.get(`${base}/api/v1/admin/customers`, ({ request }) => {
        seen.push(new URL(request.url))
        return HttpResponse.json({ items: [{ id: customerId, name: 'João Pereira', phone: '5592991234567', orders: 2 }], nextCursor: 'c2' })
      }),
    )

    await expect(listCustomers(client, { q: 'joão', cursor: 'c1', limit: 50 })).resolves.toMatchObject({ nextCursor: 'c2' })
    await listCustomers(client)

    expect(Object.fromEntries(at(seen).searchParams)).toEqual({ q: 'joão', cursor: 'c1', limit: '50' })
    expect(at(seen, 1).search).toBe('')
  })

  it('gets one customer', async () => {
    server.use(http.get(`${base}/api/v1/admin/customers/${customerId}`, () => HttpResponse.json(customer)))

    await expect(getCustomer(client, customerId)).resolves.toEqual(customer)
  })

  it('creates a customer with its addresses', async () => {
    let body: unknown
    server.use(
      http.post(`${base}/api/v1/admin/customers`, async ({ request }) => {
        body = await request.json()
        return HttpResponse.json(customer, { status: 201 })
      }),
    )
    const input = { name: 'João Pereira', phone: '5592991234567', addresses: [{ label: 'Casa', cep: '69057000', street: 'Rua A', city: 'Manaus', state: 'AM' as const }] }

    await expect(createCustomer(client, input)).resolves.toEqual(customer)
    expect(body).toEqual(input)
  })

  it('replaces a customer with the version read', async () => {
    let body: unknown
    server.use(
      http.put(`${base}/api/v1/admin/customers/${customerId}`, async ({ request }) => {
        body = await request.json()
        return HttpResponse.json({ ...customer, version: 2 })
      }),
    )

    await expect(updateCustomer(client, customerId, { name: 'João P.', phone: '5592991234567', addresses: [], version: 1 })).resolves.toMatchObject({ version: 2 })
    expect(body).toEqual({ name: 'João P.', phone: '5592991234567', addresses: [], version: 1 })
  })

  it('reports phone_taken, version_conflict, not_found and invalid fields', async () => {
    server.use(http.post(`${base}/api/v1/admin/customers`, () => problem(409, 'phone_taken')))
    const taken = await createCustomer(client, { name: 'x', phone: '5592991234567', addresses: [] }).catch((error: unknown) => error)
    expect(taken).toBeInstanceOf(ApiError)
    expect(taken).toMatchObject({ status: 409, code: 'phone_taken' })

    server.use(http.put(`${base}/api/v1/admin/customers/${customerId}`, () => problem(409, 'version_conflict')))
    await expect(updateCustomer(client, customerId, { name: 'x', phone: '5592991234567', addresses: [], version: 1 })).rejects.toMatchObject({ code: 'version_conflict' })

    server.use(http.get(`${base}/api/v1/admin/customers/${customerId}`, () => problem(404, 'not_found')))
    await expect(getCustomer(client, customerId)).rejects.toMatchObject({ status: 404, code: 'not_found' })

    server.use(
      http.post(`${base}/api/v1/admin/customers`, () =>
        HttpResponse.json(
          { status: 400, code: 'invalid', title: 'Dados inválidos', errors: [{ field: 'email', message: 'Confira o e-mail.' }] },
          { status: 400, headers: { 'Content-Type': 'application/problem+json' } },
        ),
      ),
    )
    await expect(createCustomer(client, { name: 'x', phone: '1', addresses: [] })).rejects.toMatchObject({ fieldIssues: [{ field: 'email', message: 'Confira o e-mail.' }] })
  })
})
