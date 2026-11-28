import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from 'vitest'

import { PhotoFileError, PhotoUploadError } from './catalog'
import { ApiError, createHttpClient } from './http'
import { newIdempotencyKey } from './idempotency'
import { recordReceipt, toReceive, uploadReceiptPhoto } from './receipts'

const base = 'http://api.test'
const admin = `${base}/api/v1/admin`
const storage = 'http://storage.test'
const server = setupServer()

beforeAll(() => server.listen({ onUnhandledFrame: 'error' }))
afterEach(() => {
  server.resetHandlers()
  vi.restoreAllMocks()
})
afterAll(() => server.close())

const client = createHttpClient({ baseUrl: base, session: { accessToken: () => 'access-1', refresh: async () => false, scope: () => 'ana@alta' } })

const lineId = '0190f3c4-5e1a-7b2c-8d3e-0000000000f4'
const orderItemId = '0190f3c4-5e1a-7b2c-8d3e-0000000000f3'

function problem(status: number, code: string): Response {
  return HttpResponse.json({ status, code, title: 'Problema' }, { status, headers: { 'Content-Type': 'application/problem+json' } })
}

describe('toReceive', () => {
  it('reads the groups and the items bought at the counter', async () => {
    const answer = { groups: [], toBuy: [], truncated: false }
    let authorization: string | null = null
    server.use(
      http.get(`${admin}/purchasing/to-receive`, ({ request }) => {
        authorization = request.headers.get('Authorization')
        return HttpResponse.json(answer)
      }),
    )

    await expect(toReceive(client)).resolves.toEqual(answer)
    expect(authorization).toBe('Bearer access-1')
  })

  it('maps a refusal to ApiError with its code', async () => {
    server.use(http.get(`${admin}/purchasing/to-receive`, () => problem(403, 'forbidden')))
    const failure = await toReceive(client).catch((error: unknown) => error)
    expect(failure).toBeInstanceOf(ApiError)
    expect(failure).toMatchObject({ status: 403, code: 'forbidden' })
  })
})

describe('uploadReceiptPhoto', () => {
  const key = 't/tenant/receipts/abc'
  const signed = { 'Content-Type': 'image/png', 'x-amz-meta-owner': 'receipt' }
  const file = new Blob(['png-bytes'], { type: 'image/png' })

  const issued = () =>
    HttpResponse.json({ key, uploadUrl: `${storage}/bucket/${key}?sig=abc`, method: 'PUT', headers: signed, expiresAt: '2026-11-28T12:00:00Z' }, { status: 201 })

  it('requests the upload, sends the file with the signed headers only, and resolves the key', async () => {
    const requests: unknown[] = []
    const stored: { authorization: string | null; type: string | null; owner: string | null; body: string }[] = []
    server.use(
      http.post(`${admin}/purchasing/receipt-photo-uploads`, async ({ request }) => {
        requests.push(await request.json())
        return issued()
      }),
      http.put(`${storage}/bucket/*`, async ({ request }) => {
        stored.push({
          authorization: request.headers.get('Authorization'),
          type: request.headers.get('Content-Type'),
          owner: request.headers.get('x-amz-meta-owner'),
          body: await request.text(),
        })
        return new HttpResponse(null, { status: 200 })
      }),
    )

    await expect(uploadReceiptPhoto(client, { file })).resolves.toBe(key)

    expect(requests).toEqual([{ contentType: 'image/png' }])
    expect(stored).toEqual([{ authorization: null, type: 'image/png', owner: 'receipt', body: 'png-bytes' }])
  })

  it('throws PhotoUploadError when storage refuses the PUT', async () => {
    server.use(
      http.post(`${admin}/purchasing/receipt-photo-uploads`, issued),
      http.put(`${storage}/bucket/*`, () => new HttpResponse(null, { status: 403 })),
    )
    const failure = await uploadReceiptPhoto(client, { file }).catch((error: unknown) => error)
    expect(failure).toBeInstanceOf(PhotoUploadError)
    expect(failure).toMatchObject({ status: 403 })
  })

  it.each([
    ['image/gif', 1, 'photo_type_unsupported'],
    ['image/jpeg', 10 * 1024 * 1024 + 1, 'photo_too_large'],
  ])('refuses a %s of %i bytes before any request (%s)', async (type, size, code) => {
    const failure = await uploadReceiptPhoto(client, { file: new Blob([new Uint8Array(size)], { type }) }).catch((error: unknown) => error)
    expect(failure).toBeInstanceOf(PhotoFileError)
    expect(failure).toMatchObject({ code })
  })

  it('maps the API refusal of the request (rate limit) to ApiError', async () => {
    server.use(http.post(`${admin}/purchasing/receipt-photo-uploads`, () => problem(429, 'rate_limited')))
    await expect(uploadReceiptPhoto(client, { file })).rejects.toMatchObject({ status: 429, code: 'rate_limited' })
  })
})

describe('recordReceipt', () => {
  const body = { entries: [{ lineId, check: 'defective' as const, note: 'Veio riscado', photoKeys: ['k1'] }, { orderItemId, check: 'ok' as const }], invoicePhotoKeys: ['k2'] }
  const saved = { id: 'r1', receivedAt: '2026-11-28T12:00:00Z', entries: [], completedOrders: [], backToBuy: [] }

  it('posts the entries with the idempotency key as a header', async () => {
    const seen: { body: unknown; key: string | null }[] = []
    server.use(
      http.post(`${admin}/purchasing/receipts`, async ({ request }) => {
        seen.push({ body: await request.json(), key: request.headers.get('Idempotency-Key') })
        return HttpResponse.json(saved, { status: 201 })
      }),
    )
    const key = newIdempotencyKey()

    await expect(recordReceipt(client, body, key)).resolves.toEqual(saved)
    expect(seen).toEqual([{ body, key }])
  })

  it('keeps the same key on a retry after a lost answer', async () => {
    const keys: (string | null)[] = []
    server.use(
      http.post(`${admin}/purchasing/receipts`, ({ request }) => {
        keys.push(request.headers.get('Idempotency-Key'))
        return keys.length === 1 ? HttpResponse.error() : HttpResponse.json(saved, { status: 201 })
      }),
    )
    const key = newIdempotencyKey()

    await expect(recordReceipt(client, body, key)).rejects.toMatchObject({ status: 0 })
    await expect(recordReceipt(client, body, key)).resolves.toEqual(saved)
    expect(keys).toEqual([key, key])
  })

  it.each([
    [400, 'invalid'],
    [400, 'photo_not_uploaded'],
    [409, 'line_closed'],
    [409, 'order_closed'],
    [422, 'idempotency_key_reused'],
  ])('maps %i %s to ApiError', async (status, code) => {
    server.use(http.post(`${admin}/purchasing/receipts`, () => problem(status, code)))
    await expect(recordReceipt(client, body, newIdempotencyKey())).rejects.toMatchObject({ status, code })
  })
})
