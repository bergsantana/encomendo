import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, describe, expect, it } from 'vitest'

import { ApiError, createHttpClient } from './http'
import { newIdempotencyKey } from './idempotency'
import {
  markInTransit,
  notices,
  orderPurchases,
  previewSupplierOrder,
  purchasingToday,
  recordPaidPrices,
  recordPromisedDate,
  resolveNotice,
  sendSupplierOrder,
  supplierOrder,
  supplierOrders,
  toBuy,
  undoSupplierOrder,
} from './purchasing'
import { supplierItems } from './suppliers'

/** at reads a recorded call, failing the test with a clear message when it never happened. */
function at<T>(list: readonly T[], index = 0): T {
  const found = list[index]
  if (found === undefined) throw new Error(`call ${index} never happened`)
  return found
}

const base = 'http://api.test'
const admin = `${base}/api/v1/admin`
const server = setupServer()

beforeAll(() => server.listen({ onUnhandledFrame: 'error' }))
afterEach(() => server.resetHandlers())
afterAll(() => server.close())

const client = createHttpClient({ baseUrl: base, session: { accessToken: () => 'access-1', refresh: async () => false, scope: () => 'ana@alta' } })

const supplierId = '0190f3c4-5e1a-7b2c-8d3e-0000000000f1'
const supplierOrderId = '0190f3c4-5e1a-7b2c-8d3e-0000000000f2'
const orderItemId = '0190f3c4-5e1a-7b2c-8d3e-0000000000f3'
const lineId = '0190f3c4-5e1a-7b2c-8d3e-0000000000f4'
const orderId = '0190f3c4-5e1a-7b2c-8d3e-0000000000d1'

function problem(status: number, code: string): Response {
  return HttpResponse.json({ status, code, title: 'Problema' }, { status, headers: { 'Content-Type': 'application/problem+json' } })
}

/** record answers a route and keeps what the call sent: the body and two headers. */
function record(method: 'get' | 'post' | 'put', path: string, answer: unknown, status = 200) {
  const seen: { url: URL; body: unknown; authorization: string | null; key: string | null }[] = []
  server.use(
    http[method](`${admin}${path}`, async ({ request }) => {
      const text = await request.text()
      seen.push({
        url: new URL(request.url),
        body: text === '' ? undefined : JSON.parse(text),
        authorization: request.headers.get('Authorization'),
        key: request.headers.get('Idempotency-Key'),
      })
      return status === 204 ? new HttpResponse(null, { status }) : HttpResponse.json(answer as Record<string, unknown>, { status })
    }),
  )
  return seen
}

const failureOf = (call: Promise<unknown>) => call.catch((error: unknown) => error)

describe('toBuy', () => {
  it('reads the suppliers with their items', async () => {
    const answer = { suppliers: [], truncated: false }
    const seen = record('get', '/purchasing/to-buy', answer)

    await expect(toBuy(client)).resolves.toEqual(answer)
    expect(at(seen).authorization).toBe('Bearer access-1')
  })

  it('maps a refusal to ApiError with its code', async () => {
    server.use(http.get(`${admin}/purchasing/to-buy`, () => problem(403, 'forbidden')))
    const failure = await failureOf(toBuy(client))
    expect(failure).toBeInstanceOf(ApiError)
    expect(failure).toMatchObject({ status: 403, code: 'forbidden' })
  })
})

describe('previewSupplierOrder', () => {
  const body = { supplierId, orderItemIds: [orderItemId] }

  it('posts the picks and returns the text, with no idempotency key', async () => {
    const seen = record('post', '/purchasing/supplier-orders/preview', { text: 'Olá!' })

    await expect(previewSupplierOrder(client, body)).resolves.toEqual({ text: 'Olá!' })
    expect(at(seen)).toMatchObject({ body, key: null })
  })

  it('throws item_not_to_buy for a pick that is no longer in "A comprar"', async () => {
    server.use(http.post(`${admin}/purchasing/supplier-orders/preview`, () => problem(409, 'item_not_to_buy')))
    await expect(previewSupplierOrder(client, body)).rejects.toMatchObject({ status: 409, code: 'item_not_to_buy' })
  })
})

describe('sendSupplierOrder', () => {
  const body = { supplierId, orderItemIds: [orderItemId] }
  const sent = { supplierOrder: { id: supplierOrderId, number: 'C-0012' }, whatsappUrl: 'https://wa.me/5511999990000?text=x', text: 'x' }

  it('sends the idempotency key as a header and returns the link', async () => {
    const seen = record('post', '/purchasing/supplier-orders', sent, 201)
    const key = newIdempotencyKey()

    await expect(sendSupplierOrder(client, body, key)).resolves.toEqual(sent)
    expect(at(seen)).toMatchObject({ body, key })
  })

  it('keeps the same key on a retry after a lost answer', async () => {
    let attempts = 0
    const keys: (string | null)[] = []
    server.use(
      http.post(`${admin}/purchasing/supplier-orders`, ({ request }) => {
        keys.push(request.headers.get('Idempotency-Key'))
        attempts += 1
        return attempts === 1 ? HttpResponse.error() : HttpResponse.json(sent, { status: 201 })
      }),
    )
    const key = newIdempotencyKey()

    await expect(sendSupplierOrder(client, body, key)).rejects.toMatchObject({ status: 0 })
    await expect(sendSupplierOrder(client, body, key)).resolves.toEqual(sent)
    expect(keys).toEqual([key, key])
  })

  it.each([
    [409, 'item_not_to_buy'],
    [422, 'idempotency_key_reused'],
  ])('maps %s %s to ApiError', async (status, code) => {
    server.use(http.post(`${admin}/purchasing/supplier-orders`, () => problem(status, code)))
    await expect(sendSupplierOrder(client, body, newIdempotencyKey())).rejects.toMatchObject({ status, code })
  })
})

describe('supplierOrders', () => {
  it('lists with the filter, cursor and limit', async () => {
    const seen = record('get', '/purchasing/supplier-orders', { items: [], nextCursor: 'n1' })

    await expect(supplierOrders(client, { state: 'all', cursor: 'c1', limit: 30 })).resolves.toEqual({ items: [], nextCursor: 'n1' })
    expect(Object.fromEntries(at(seen).url.searchParams)).toEqual({ state: 'all', cursor: 'c1', limit: '30' })
  })

  it('asks for the first page with no query', async () => {
    const seen = record('get', '/purchasing/supplier-orders', { items: [] })
    await supplierOrders(client)
    expect(at(seen).url.search).toBe('')
  })
})

describe('supplierOrder', () => {
  it('reads one pedido de compra', async () => {
    const answer = { id: supplierOrderId, number: 'C-0012', version: 1 }
    record('get', `/purchasing/supplier-orders/${supplierOrderId}`, answer)
    await expect(supplierOrder(client, supplierOrderId)).resolves.toEqual(answer)
  })

  it('throws not_found for an unknown id', async () => {
    server.use(http.get(`${admin}/purchasing/supplier-orders/${supplierOrderId}`, () => problem(404, 'not_found')))
    await expect(supplierOrder(client, supplierOrderId)).rejects.toMatchObject({ status: 404, code: 'not_found' })
  })
})

describe('recordPromisedDate', () => {
  const body = { version: 1, date: '2026-12-03', lines: [{ lineId, date: '2026-12-05' }] }

  it('puts the date, the per-line dates and the version', async () => {
    const seen = record('put', `/purchasing/supplier-orders/${supplierOrderId}/promised-date`, { id: supplierOrderId, version: 2 })

    await expect(recordPromisedDate(client, supplierOrderId, body)).resolves.toMatchObject({ version: 2 })
    expect(at(seen).body).toEqual(body)
  })

  it.each([
    [400, 'invalid'],
    [409, 'version_conflict'],
  ])('maps %s %s to ApiError', async (status, code) => {
    server.use(http.put(`${admin}/purchasing/supplier-orders/${supplierOrderId}/promised-date`, () => problem(status, code)))
    await expect(recordPromisedDate(client, supplierOrderId, body)).rejects.toMatchObject({ status, code })
  })
})

describe('markInTransit and undoSupplierOrder', () => {
  it('posts the version to /in-transit', async () => {
    const seen = record('post', `/purchasing/supplier-orders/${supplierOrderId}/in-transit`, { id: supplierOrderId, version: 3 })
    await expect(markInTransit(client, supplierOrderId, { version: 2 })).resolves.toMatchObject({ version: 3 })
    expect(at(seen).body).toEqual({ version: 2 })
  })

  it('posts the version to /undo', async () => {
    const seen = record('post', `/purchasing/supplier-orders/${supplierOrderId}/undo`, { id: supplierOrderId, state: 'undone' })
    await expect(undoSupplierOrder(client, supplierOrderId, { version: 2 })).resolves.toMatchObject({ state: 'undone' })
    expect(at(seen).body).toEqual({ version: 2 })
  })

  it('throws supplier_order_received when something was received', async () => {
    server.use(http.post(`${admin}/purchasing/supplier-orders/${supplierOrderId}/undo`, () => problem(409, 'supplier_order_received')))
    await expect(undoSupplierOrder(client, supplierOrderId, { version: 2 })).rejects.toMatchObject({ status: 409, code: 'supplier_order_received' })
  })
})

describe('recordPaidPrices', () => {
  const body = { version: 2, prices: [{ lineId, cents: 8990 }] }

  it('puts the prices in cents and the version', async () => {
    const seen = record('put', `/purchasing/supplier-orders/${supplierOrderId}/paid-prices`, { id: supplierOrderId, version: 3 })
    await expect(recordPaidPrices(client, supplierOrderId, body)).resolves.toMatchObject({ version: 3 })
    expect(at(seen).body).toEqual(body)
  })

  it.each([
    [403, 'forbidden'],
    [409, 'line_closed'],
  ])('maps %s %s to ApiError', async (status, code) => {
    server.use(http.put(`${admin}/purchasing/supplier-orders/${supplierOrderId}/paid-prices`, () => problem(status, code)))
    await expect(recordPaidPrices(client, supplierOrderId, body)).rejects.toMatchObject({ status, code })
  })
})

describe('notices', () => {
  it('lists the notices', async () => {
    record('get', '/purchasing/notices', { items: [] })
    await expect(notices(client)).resolves.toEqual({ items: [] })
  })

  it('resolves one by its line and answers nothing', async () => {
    const seen = record('post', `/purchasing/notices/${lineId}/resolve`, undefined, 204)
    await expect(resolveNotice(client, lineId)).resolves.toBeUndefined()
    expect(at(seen)).toMatchObject({ body: undefined, authorization: 'Bearer access-1' })
  })

  it('throws line_closed for a line that is no longer a notice', async () => {
    server.use(http.post(`${admin}/purchasing/notices/${lineId}/resolve`, () => problem(409, 'line_closed')))
    await expect(resolveNotice(client, lineId)).rejects.toMatchObject({ status: 409, code: 'line_closed' })
  })
})

describe('orderPurchases and purchasingToday', () => {
  it("reads one order's purchases", async () => {
    record('get', `/purchasing/orders/${orderId}/lines`, { items: [] })
    await expect(orderPurchases(client, orderId)).resolves.toEqual({ items: [] })
  })

  it("reads Hoje's purchasing entries", async () => {
    const answer = { toSend: 2, needs: [] }
    record('get', '/purchasing/today', answer)
    await expect(purchasingToday(client)).resolves.toEqual(answer)
  })
})

describe('supplierItems', () => {
  it("reads the supplier's items", async () => {
    const answer = { items: [{ itemId: orderItemId, itemName: 'Banco', role: 'main', leadDays: 5 }] }
    record('get', `/suppliers/${supplierId}/items`, answer)
    await expect(supplierItems(client, supplierId)).resolves.toEqual(answer)
  })

  it('throws not_found for an unknown supplier', async () => {
    server.use(http.get(`${admin}/suppliers/${supplierId}/items`, () => problem(404, 'not_found')))
    await expect(supplierItems(client, supplierId)).rejects.toMatchObject({ status: 404, code: 'not_found' })
  })
})
