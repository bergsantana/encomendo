import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, describe, expect, it } from 'vitest'

import { ApiError, createHttpClient } from './http'
import { newIdempotencyKey } from './idempotency'
import {
  advanceOrder,
  cancelOrder,
  changeDeliveryDate,
  changeOrder,
  createOrder,
  getFinanceSummary,
  getOrder,
  getToday,
  listOrders,
  moveOrderItem,
  orderHistory,
  quoteOrder,
  recordPayment,
  releaseOrder,
  sendTrackingLink,
  useBackupSupplier,
  voidPayment,
} from './orders'

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

const orderId = '0190f3c4-5e1a-7b2c-8d3e-0000000000d1'
const itemId = '0190f3c4-5e1a-7b2c-8d3e-0000000000d2'
const paymentId = '0190f3c4-5e1a-7b2c-8d3e-0000000000d3'
const customerId = '0190f3c4-5e1a-7b2c-8d3e-0000000000c1'
const order = { id: orderId, number: 'P-0231', version: 2 }

function problem(status: number, code: string, extra: Record<string, unknown> = {}): Response {
  return HttpResponse.json({ status, code, title: 'Problema', ...extra }, { status, headers: { 'Content-Type': 'application/problem+json' } })
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
      return HttpResponse.json(answer as Record<string, unknown>, { status })
    }),
  )
  return seen
}

const terms = { handover: 'pickup' as const }
const lines = [{ itemId, quantity: 2 }]

describe('quoteOrder', () => {
  it('posts the lines and terms and returns what the server computed', async () => {
    const quote = { subtotal: { cents: 90000, currency: 'BRL' }, total: { cents: 90000, currency: 'BRL' }, deliveryDate: '2026-12-12' }
    const seen = record('post', '/orders/quote', quote)

    await expect(quoteOrder(client, { lines, terms, orderId })).resolves.toEqual(quote)
    expect(seen[0]).toMatchObject({ body: { lines, terms, orderId }, authorization: 'Bearer access-1' })
  })

  it('maps its refusals to ApiError with their code', async () => {
    server.use(http.post(`${admin}/orders/quote`, () => problem(409, 'item_unavailable', { references: [{ id: itemId, name: 'Banco' }] })))

    const failure = await quoteOrder(client, { lines, terms }).catch((error: unknown) => error)
    expect(failure).toBeInstanceOf(ApiError)
    expect(failure).toMatchObject({ status: 409, code: 'item_unavailable', references: [{ id: itemId, name: 'Banco' }] })
  })
})

describe('createOrder', () => {
  const body = { customerId, lines, terms, sendToCustomer: false }

  it('sends the idempotency key as a header and returns the tracking link', async () => {
    const created = { order, trackingUrl: 'https://altarpm.encomendo.com.br/p/token' }
    const seen = record('post', '/orders', created, 201)
    const key = newIdempotencyKey()

    await expect(createOrder(client, body, key)).resolves.toEqual(created)
    expect(seen[0]).toMatchObject({ body, key })
  })

  it('keeps the idempotency key on retry', async () => {
    let attempts = 0
    const keys: (string | null)[] = []
    server.use(
      http.post(`${admin}/orders`, ({ request }) => {
        keys.push(request.headers.get('Idempotency-Key'))
        attempts += 1
        // The first answer is lost on the way, as on a flaky phone connection.
        return attempts === 1 ? HttpResponse.error() : HttpResponse.json({ order }, { status: 201 })
      }),
    )
    const key = newIdempotencyKey()

    await expect(createOrder(client, body, key)).rejects.toMatchObject({ status: 0, code: 'network' })
    await expect(createOrder(client, body, key)).resolves.toEqual({ order })

    expect(keys).toEqual([key, key])
  })

  it('makes a new key for a new action', () => {
    expect(newIdempotencyKey()).not.toBe(newIdempotencyKey())
  })

  it.each([
    [400, 'idempotency_key_required'],
    [400, 'invalid'],
    [404, 'not_found'],
    [409, 'item_unavailable'],
    [409, 'conflict'],
    [422, 'idempotency_key_reused'],
  ])('maps %i %s to ApiError', async (status, code) => {
    server.use(http.post(`${admin}/orders`, () => problem(status, code)))

    await expect(createOrder(client, body, 'k1')).rejects.toMatchObject({ status, code })
  })
})

describe('listOrders', () => {
  it('sends the filter and leaves out what is not set, including alert=false', async () => {
    const seen = record('get', '/orders', { items: [], nextCursor: 'c2' })

    await listOrders(client, { customerId, scope: 'all', status: 'em_montagem', phase: 'assembling', q: 'P-0231', alert: true, cursor: 'c1', limit: 20 })
    await listOrders(client, { alert: false })
    await listOrders(client)

    expect(Object.fromEntries(at(seen).url.searchParams)).toEqual({
      customerId,
      scope: 'all',
      status: 'em_montagem',
      phase: 'assembling',
      q: 'P-0231',
      alert: 'true',
      cursor: 'c1',
      limit: '20',
    })
    expect(at(seen, 1).url.search).toBe('')
    expect(at(seen, 2).url.search).toBe('')
  })
})

describe('one order', () => {
  it('reads it', async () => {
    record('get', `/orders/${orderId}`, order)

    await expect(getOrder(client, orderId)).resolves.toEqual(order)
  })

  it('changes it with the lines, terms, approval and version', async () => {
    const answer = { order, change: { paid: true }, whatsappUrl: 'https://wa.me/5592991234567?text=Ol%C3%A1' }
    const seen = record('put', `/orders/${orderId}`, answer)
    const body = { lines, terms, customerApproved: true, version: 2 }

    await expect(changeOrder(client, orderId, body)).resolves.toEqual(answer)
    expect(at(seen).body).toEqual(body)
  })

  it('passes the wa.me link through as the API wrote it', async () => {
    const url = 'https://wa.me/5592991234567?text=Ol%C3%A1%2C%20Jo%C3%A3o'
    record('put', `/orders/${orderId}/delivery-date`, { order, whatsappUrl: url })

    const answer = await changeDeliveryDate(client, orderId, { date: '2026-12-20', reason: 'Atraso do fornecedor', version: 2 })

    expect(answer.whatsappUrl).toBe(url)
  })

  it('reads the history with its page', async () => {
    const seen = record('get', `/orders/${orderId}/history`, { items: [], nextCursor: 'h2' })

    await expect(orderHistory(client, orderId, { cursor: 'h1', limit: 10 })).resolves.toEqual({ items: [], nextCursor: 'h2' })
    await orderHistory(client, orderId)

    expect(Object.fromEntries(at(seen).url.searchParams)).toEqual({ cursor: 'h1', limit: '10' })
    expect(at(seen, 1).url.search).toBe('')
  })

  it('issues a tracking link', async () => {
    const link = { trackingUrl: 'https://altarpm.encomendo.com.br/p/novo', whatsappUrl: 'https://wa.me/5592991234567?text=x' }
    const seen = record('post', `/orders/${orderId}/tracking-link`, link)

    await expect(sendTrackingLink(client, orderId)).resolves.toEqual(link)
    expect(at(seen).body).toBeUndefined()
  })

  it('moves, releases, cancels and switches supplier with the version read', async () => {
    const advance = record('post', `/orders/${orderId}/advance`, order)
    const release = record('post', `/orders/${orderId}/release`, order)
    const cancel = record('post', `/orders/${orderId}/cancel`, order)
    const move = record('post', `/orders/${orderId}/items/${itemId}/move`, order)
    const backup = record('post', `/orders/${orderId}/items/${itemId}/use-backup-supplier`, order)

    await advanceOrder(client, orderId, { to: 'entregue', handover: 'delivery', version: 2 })
    await releaseOrder(client, orderId, { version: 2 })
    await cancelOrder(client, orderId, { reason: 'Desistiu', version: 2 })
    await moveOrderItem(client, orderId, itemId, { to: 'received', check: 'ok', version: 2 })
    await useBackupSupplier(client, orderId, itemId, { version: 2 })

    expect(at(advance).body).toEqual({ to: 'entregue', handover: 'delivery', version: 2 })
    expect(at(release).body).toEqual({ version: 2 })
    expect(at(cancel).body).toEqual({ reason: 'Desistiu', version: 2 })
    expect(at(move).body).toEqual({ to: 'received', check: 'ok', version: 2 })
    expect(at(backup).body).toEqual({ version: 2 })
  })

  it('voids a payment with the reason', async () => {
    const seen = record('post', `/orders/${orderId}/payments/${paymentId}/void`, order)

    await voidPayment(client, orderId, paymentId, { reason: 'Valor errado', version: 3 })

    expect(at(seen).body).toEqual({ reason: 'Valor errado', version: 3 })
  })

  it.each([
    ['guard_failed', 409],
    ['invalid_transition', 409],
    ['order_not_editable', 409],
    ['order_closed', 409],
    ['invalid_item_move', 409],
    ['no_backup_supplier', 409],
    ['version_conflict', 409],
    ['approval_required', 400],
    ['payment_exceeds_due', 400],
    ['invalid', 400],
    ['forbidden', 403],
    ['not_found', 404],
  ])('maps %s to an ApiError with status %i', async (code, status) => {
    server.use(
      http.get(`${admin}/orders/${orderId}`, () => problem(status, code)),
      http.put(`${admin}/orders/${orderId}`, () => problem(status, code)),
      http.post(`${admin}/orders/${orderId}/advance`, () => problem(status, code)),
      http.post(`${admin}/orders/${orderId}/cancel`, () => problem(status, code)),
      http.post(`${admin}/orders/${orderId}/items/${itemId}/move`, () => problem(status, code)),
      http.post(`${admin}/orders/${orderId}/items/${itemId}/use-backup-supplier`, () => problem(status, code)),
    )
    const calls = [
      getOrder(client, orderId),
      changeOrder(client, orderId, { lines, terms, customerApproved: false, version: 1 }),
      advanceOrder(client, orderId, { to: 'x', version: 1 }),
      cancelOrder(client, orderId, { reason: 'x', version: 1 }),
      moveOrderItem(client, orderId, itemId, { to: 'received', version: 1 }),
      useBackupSupplier(client, orderId, itemId, { version: 1 }),
    ]

    for (const call of calls) await expect(call).rejects.toMatchObject({ status, code })
  })

  it('names the guard that did not hold', async () => {
    server.use(http.post(`${admin}/orders/${orderId}/advance`, () => problem(409, 'guard_failed', { guard: 'deposit_unpaid' })))

    const failure = await advanceOrder(client, orderId, { to: 'x', version: 1 }).catch((error: unknown) => error)
    expect(failure).toBeInstanceOf(ApiError)
    expect(failure).toMatchObject({ code: 'guard_failed', guard: 'deposit_unpaid' })
  })
})

describe('recordPayment', () => {
  const body = { kind: 'deposit' as const, amountCents: 30000, method: 'pix' as const, version: 2 }

  it('sends the idempotency key as a header', async () => {
    const seen = record('post', `/orders/${orderId}/payments`, order, 201)

    await expect(recordPayment(client, orderId, body, 'key-1')).resolves.toEqual(order)
    expect(seen[0]).toMatchObject({ body, key: 'key-1' })
  })

  it('keeps the idempotency key on retry', async () => {
    const keys: (string | null)[] = []
    server.use(
      http.post(`${admin}/orders/${orderId}/payments`, ({ request }) => {
        keys.push(request.headers.get('Idempotency-Key'))
        return keys.length === 1 ? HttpResponse.error() : HttpResponse.json(order, { status: 201 })
      }),
    )
    const key = newIdempotencyKey()

    await expect(recordPayment(client, orderId, body, key)).rejects.toMatchObject({ code: 'network' })
    await expect(recordPayment(client, orderId, body, key)).resolves.toEqual(order)

    expect(keys).toEqual([key, key])
  })

  it.each([
    [400, 'payment_exceeds_due'],
    [400, 'idempotency_key_required'],
    [409, 'version_conflict'],
    [409, 'conflict'],
    [422, 'idempotency_key_reused'],
  ])('maps %i %s to ApiError', async (status, code) => {
    server.use(http.post(`${admin}/orders/${orderId}/payments`, () => problem(status, code)))

    await expect(recordPayment(client, orderId, body, 'k1')).rejects.toMatchObject({ status, code })
  })
})

describe('Hoje and Financeiro', () => {
  it('reads Hoje', async () => {
    const today = { truncated: false, today: '2026-11-27', counts: { active: 3, awaitingDeposit: 1, withAlert: 0, dueIn7Days: 2 }, needsYou: [], upcoming: [] }
    record('get', '/today', today)

    await expect(getToday(client)).resolves.toEqual(today)
  })

  it('reads the finance summary for a period', async () => {
    const seen = record('get', '/finance/summary', { truncated: false })

    await getFinanceSummary(client, { from: '2026-11-01', to: '2026-11-30' })

    expect(Object.fromEntries(at(seen).url.searchParams)).toEqual({ from: '2026-11-01', to: '2026-11-30' })
  })

  it('maps a period that is too long and a role without finance access', async () => {
    server.use(http.get(`${admin}/finance/summary`, () => problem(400, 'invalid')))
    await expect(getFinanceSummary(client, { from: '2025-01-01', to: '2026-11-30' })).rejects.toMatchObject({ status: 400, code: 'invalid' })

    server.use(http.get(`${admin}/finance/summary`, () => problem(403, 'forbidden')))
    await expect(getFinanceSummary(client, { from: '2026-11-01', to: '2026-11-30' })).rejects.toMatchObject({ status: 403, code: 'forbidden' })
  })
})
