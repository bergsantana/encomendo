import { delay, http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from 'vitest'

import {
  archiveItem,
  archiveKit,
  confirmItemPhoto,
  confirmKitPhoto,
  createAttribute,
  createCategory,
  createItem,
  createKit,
  deleteAttribute,
  deleteCategory,
  deleteItemPhoto,
  deleteKitPhoto,
  getItem,
  getKit,
  listAttributes,
  listCategories,
  listItems,
  listKits,
  orderItemPhotos,
  orderKitPhotos,
  PhotoFileError,
  PhotoUploadError,
  replaceAttribute,
  replaceItem,
  replaceKit,
  requestPhotoUpload,
  restoreItem,
  restoreKit,
  setItemPublished,
  setKitPublished,
  updateCategory,
  uploadPhoto,
} from './catalog'
import { ApiError, createHttpClient } from './http'

const base = 'http://api.test'
const storage = 'http://storage.test'
const server = setupServer()

beforeAll(() => server.listen({ onUnhandledFrame: 'error' }))
afterEach(() => {
  server.resetHandlers()
  vi.restoreAllMocks()
})
afterAll(() => server.close())

const client = createHttpClient({ baseUrl: base, session: { accessToken: () => 'access-1', refresh: async () => false, scope: () => 'ana@alta' } })

const itemId = '0190f3c4-5e1a-7b2c-8d3e-0000000000a1'
const kitId = '0190f3c4-5e1a-7b2c-8d3e-0000000000b1'
const photoId = '0190f3c4-5e1a-7b2c-8d3e-0000000000c1'
const categoryId = '0190f3c4-5e1a-7b2c-8d3e-0000000000d1'
const attributeId = '0190f3c4-5e1a-7b2c-8d3e-0000000000e1'
const admin = '/api/v1/admin/catalog'

interface Seen {
  method: string
  path: string
  query: string
  body: unknown
  authorization: string | null
}

/** answer serves every admin route with one response and records what arrived. */
function answer(status: number, body?: unknown): Seen[] {
  const seen: Seen[] = []
  server.use(
    http.all(`${base}/api/v1/admin/*`, async ({ request }) => {
      const url = new URL(request.url)
      const text = await request.text()
      seen.push({
        method: request.method,
        path: url.pathname,
        query: url.search,
        body: text === '' ? undefined : (JSON.parse(text) as unknown),
        authorization: request.headers.get('Authorization'),
      })
      return body === undefined ? new HttpResponse(null, { status }) : HttpResponse.json(body, { status })
    }),
  )
  return seen
}

function problemResponse(status: number, code: string): Response {
  return HttpResponse.json({ status, code, title: 'Problema' }, { status, headers: { 'Content-Type': 'application/problem+json' } })
}

/** problem makes every admin route answer with an RFC 9457 body carrying `code`. */
function problem(status: number, code: string): void {
  server.use(http.all(`${base}/api/v1/admin/*`, () => problemResponse(status, code)))
}

async function codeOf(call: Promise<unknown>): Promise<string | undefined> {
  try {
    await call
  } catch (error) {
    return error instanceof ApiError ? error.code : undefined
  }
  return undefined
}

const photo = { id: photoId, url: 'https://cdn.test/p.jpg', expiresAt: '2026-11-27T12:00:00Z', position: 0 }
const item = { id: itemId, name: 'Capacete', version: 3 }
const kit = { id: kitId, name: 'Kit básico', version: 2 }
const itemBody = {
  name: 'Capacete',
  description: '',
  categoryId: null,
  sourcingMode: 'to_order' as const,
  priceCents: 15000,
  attributeValueIds: [],
  options: [],
  published: false,
}
const kitBody = {
  name: 'Kit básico',
  description: '',
  priceCents: 40000,
  swapDiscount: 'keep' as const,
  assemblyDays: null,
  safetyMarginDays: null,
  items: [{ itemId, quantity: 1, swappable: false, alternativeItemIds: [] }],
  attributeValueIds: [],
  published: false,
}

describe('categories', () => {
  it('lists, creates, updates and deletes with the access token', async () => {
    const seen = answer(200, { items: [] })
    await expect(listCategories(client)).resolves.toEqual({ items: [] })
    await createCategory(client, { name: 'Capacetes' })
    await updateCategory(client, categoryId, { name: 'Viseiras', position: 2 })
    await deleteCategory(client, categoryId)

    expect(seen).toEqual([
      { method: 'GET', path: `${admin}/categories`, query: '', body: undefined, authorization: 'Bearer access-1' },
      { method: 'POST', path: `${admin}/categories`, query: '', body: { name: 'Capacetes' }, authorization: 'Bearer access-1' },
      { method: 'PATCH', path: `${admin}/categories/${categoryId}`, query: '', body: { name: 'Viseiras', position: 2 }, authorization: 'Bearer access-1' },
      { method: 'DELETE', path: `${admin}/categories/${categoryId}`, query: '', body: undefined, authorization: 'Bearer access-1' },
    ])
  })

  it('surfaces category_in_use', async () => {
    problem(409, 'category_in_use')
    expect(await codeOf(deleteCategory(client, categoryId))).toBe('category_in_use')
  })
})

describe('attributes', () => {
  it('lists, creates, replaces and deletes', async () => {
    const seen = answer(200, { items: [] })
    const input = { name: 'Serve em', values: [{ id: photoId, label: 'CG 160' }, { label: 'Fan' }] }
    await listAttributes(client)
    await createAttribute(client, input)
    await replaceAttribute(client, attributeId, input)
    await deleteAttribute(client, attributeId)

    expect(seen.map((call) => [call.method, call.path, call.body])).toEqual([
      ['GET', `${admin}/attributes`, undefined],
      ['POST', `${admin}/attributes`, input],
      ['PUT', `${admin}/attributes/${attributeId}`, input],
      ['DELETE', `${admin}/attributes/${attributeId}`, undefined],
    ])
  })

  it('surfaces name_taken', async () => {
    problem(409, 'name_taken')
    expect(await codeOf(createAttribute(client, { name: 'Serve em', values: [] }))).toBe('name_taken')
  })
})

describe('items', () => {
  it('lists with every filter in the query', async () => {
    const seen = answer(200, { items: [], nextCursor: 'n1' })
    await expect(listItems(client, { cursor: 'c1', limit: 25, q: 'capa', categoryId, includeArchived: true })).resolves.toEqual({
      items: [],
      nextCursor: 'n1',
    })
    expect(seen[0]?.method).toBe('GET')
    expect(seen[0]?.path).toBe(`${admin}/items`)
    expect(Object.fromEntries(new URLSearchParams(seen[0]?.query))).toEqual({
      cursor: 'c1',
      limit: '25',
      q: 'capa',
      categoryId,
      includeArchived: 'true',
    })
  })

  it('sends no query for the first page', async () => {
    const seen = answer(200, { items: [] })
    await listItems(client)
    expect(seen[0]?.query).toBe('')
  })

  it('creates, reads and publishes', async () => {
    const seen = answer(200, item)
    await createItem(client, itemBody)
    await expect(getItem(client, itemId)).resolves.toEqual(item)
    await setItemPublished(client, itemId, { published: true, version: 3 })

    expect(seen.map((call) => [call.method, call.path, call.body])).toEqual([
      ['POST', `${admin}/items`, itemBody],
      ['GET', `${admin}/items/${itemId}`, undefined],
      ['PUT', `${admin}/items/${itemId}/published`, { published: true, version: 3 }],
    ])
  })

  it('archives and restores with the version read', async () => {
    const seen = answer(200, item)
    await archiveItem(client, itemId, { version: 3 })
    await restoreItem(client, itemId, { version: 4 })

    expect(seen.map((call) => [call.method, call.path, call.body])).toEqual([
      ['POST', `${admin}/items/${itemId}/archive`, { version: 3 }],
      ['POST', `${admin}/items/${itemId}/restore`, { version: 4 }],
    ])
  })

  it('replaces without any cost key when the caller leaves the cost out', async () => {
    const seen = answer(200, item)
    await replaceItem(client, itemId, { ...itemBody, mainSupplier: { supplierId: kitId, leadDays: 5 }, version: 3 })

    const body = seen[0]?.body
    expect(seen[0]?.method).toBe('PUT')
    expect(seen[0]?.path).toBe(`${admin}/items/${itemId}`)
    expect(body).toMatchObject({ version: 3, priceCents: 15000 })
    expect(Object.keys(body as object)).not.toContain('productionCostCents')
    expect(JSON.stringify(body)).not.toContain('costCents')
  })

  it('sends a cost as given and null to clear it', async () => {
    const seen = answer(200, item)
    await replaceItem(client, itemId, {
      ...itemBody,
      productionCostCents: null,
      mainSupplier: { supplierId: kitId, leadDays: 5, costCents: 900 },
      version: 3,
    })

    expect(seen[0]?.body).toMatchObject({ productionCostCents: null, mainSupplier: { costCents: 900 } })
  })

  it('passes the owner-only fields through as sent, absent included', async () => {
    answer(200, item)
    const result = await getItem(client, itemId)
    expect('cost' in result).toBe(false)
    expect('margin' in result).toBe(false)

    answer(200, { ...item, cost: null, margin: { cents: 5, basisPoints: 100 } })
    const owner = await getItem(client, itemId)
    expect(owner.cost).toBeNull()
    expect(owner.margin).toEqual({ cents: 5, basisPoints: 100 })
  })

  it.each([
    ['version_conflict', 409, () => replaceItem(client, itemId, { ...itemBody, version: 1 })],
    ['item_in_kit', 409, () => archiveItem(client, itemId, { version: 1 })],
    ['forbidden', 403, () => createItem(client, { ...itemBody, productionCostCents: null })],
    ['not_found', 404, () => getItem(client, itemId)],
  ])('surfaces %s', async (code, status, call) => {
    problem(status, code)
    expect(await codeOf(call())).toBe(code)
  })
})

describe('kits', () => {
  it('lists without a category filter', async () => {
    const seen = answer(200, { items: [] })
    await listKits(client, { q: 'kit', limit: 10, includeArchived: false })
    expect(seen[0]?.path).toBe(`${admin}/kits`)
    expect(Object.fromEntries(new URLSearchParams(seen[0]?.query))).toEqual({ q: 'kit', limit: '10', includeArchived: 'false' })
  })

  it('creates, reads, replaces, publishes, archives and restores', async () => {
    const seen = answer(200, kit)
    await createKit(client, kitBody)
    await getKit(client, kitId)
    await replaceKit(client, kitId, { ...kitBody, version: 2 })
    await setKitPublished(client, kitId, { published: true, version: 2 })
    await archiveKit(client, kitId, { version: 2 })
    await restoreKit(client, kitId, { version: 3 })

    expect(seen.map((call) => [call.method, call.path, call.body])).toEqual([
      ['POST', `${admin}/kits`, kitBody],
      ['GET', `${admin}/kits/${kitId}`, undefined],
      ['PUT', `${admin}/kits/${kitId}`, { ...kitBody, version: 2 }],
      ['PUT', `${admin}/kits/${kitId}/published`, { published: true, version: 2 }],
      ['POST', `${admin}/kits/${kitId}/archive`, { version: 2 }],
      ['POST', `${admin}/kits/${kitId}/restore`, { version: 3 }],
    ])
  })

  it('surfaces item_archived', async () => {
    problem(409, 'item_archived')
    expect(await codeOf(createKit(client, kitBody))).toBe('item_archived')
  })
})

describe('photos', () => {
  it('confirms, orders and deletes an item photo', async () => {
    const confirmed = answer(201, photo)
    await expect(confirmItemPhoto(client, itemId, 'k1')).resolves.toEqual(photo)
    const ordered = answer(200, { items: [photo] })
    await orderItemPhotos(client, itemId, [photoId])
    const deleted = answer(204)
    await deleteItemPhoto(client, itemId, photoId)

    expect(confirmed[0]).toMatchObject({ method: 'POST', path: `${admin}/items/${itemId}/photos`, body: { key: 'k1' } })
    expect(ordered[0]).toMatchObject({ method: 'PUT', path: `${admin}/items/${itemId}/photos/order`, body: { photoIds: [photoId] } })
    expect(deleted[0]).toMatchObject({ method: 'DELETE', path: `${admin}/items/${itemId}/photos/${photoId}` })
  })

  it('confirms, orders and deletes a kit photo', async () => {
    const seen = answer(200, photo)
    await confirmKitPhoto(client, kitId, 'k2')
    await orderKitPhotos(client, kitId, [photoId])
    await deleteKitPhoto(client, kitId, photoId)

    expect(seen.map((call) => [call.method, call.path, call.body])).toEqual([
      ['POST', `${admin}/kits/${kitId}/photos`, { key: 'k2' }],
      ['PUT', `${admin}/kits/${kitId}/photos/order`, { photoIds: [photoId] }],
      ['DELETE', `${admin}/kits/${kitId}/photos/${photoId}`, undefined],
    ])
  })

  it('requests an upload and surfaces photo_limit', async () => {
    const seen = answer(201, { key: 'k', uploadUrl: `${storage}/k`, method: 'PUT', headers: {}, expiresAt: 'x' })
    await requestPhotoUpload(client, { ownerType: 'kit', ownerId: kitId, contentType: 'image/webp' })
    expect(seen[0]).toMatchObject({
      method: 'POST',
      path: `${admin}/photo-uploads`,
      body: { ownerType: 'kit', ownerId: kitId, contentType: 'image/webp' },
    })

    problem(409, 'photo_limit')
    expect(await codeOf(requestPhotoUpload(client, { ownerType: 'kit', ownerId: kitId, contentType: 'image/webp' }))).toBe('photo_limit')
  })
})

describe('uploadPhoto', () => {
  const key = `t/tenant/items/${itemId}/${photoId}`
  const signed = { 'Content-Type': 'image/png', 'x-amz-meta-owner': 'item' }
  const file = new Blob(['png-bytes'], { type: 'image/png' })
  const input = { ownerType: 'item' as const, ownerId: itemId, file }

  function uploadResponse(): Response {
    return HttpResponse.json(
      { key, uploadUrl: `${storage}/bucket/${key}?sig=abc`, method: 'PUT', headers: signed, expiresAt: '2026-11-27T12:00:00Z' },
      { status: 201 },
    )
  }

  /** serveUpload answers the three steps of an item photo upload and records their order. */
  function serveUpload(options: { storageStatus?: number; confirm?: () => Response }) {
    const calls: string[] = []
    const stored: { authorization: string | null; headers: Record<string, string>; body: string }[] = []
    server.use(
      http.post(`${base}${admin}/photo-uploads`, () => {
        calls.push('request')
        return uploadResponse()
      }),
      http.put(`${storage}/bucket/*`, async ({ request }) => {
        calls.push('put')
        stored.push({
          authorization: request.headers.get('Authorization'),
          headers: {
            'Content-Type': request.headers.get('Content-Type') ?? '',
            'x-amz-meta-owner': request.headers.get('x-amz-meta-owner') ?? '',
          },
          body: await request.text(),
        })
        return new HttpResponse(null, { status: options.storageStatus ?? 200 })
      }),
      http.post(`${base}${admin}/items/${itemId}/photos`, () => {
        calls.push('confirm')
        return options.confirm?.() ?? HttpResponse.json(photo, { status: 201 })
      }),
    )
    return { calls, stored }
  }

  it('sends the signed headers and no Authorization to storage, then confirms', async () => {
    const { calls, stored } = serveUpload({})
    // Watches the storage call's fetch options; MSW's Request does not expose credentials.
    const realFetch = globalThis.fetch
    const storageInits: (RequestInit | undefined)[] = []
    vi.spyOn(globalThis, 'fetch').mockImplementation((request, init) => {
      if (String(request).startsWith(storage)) storageInits.push(init)
      return realFetch(request, init)
    })

    await expect(uploadPhoto(client, input)).resolves.toEqual(photo)

    expect(calls).toEqual(['request', 'put', 'confirm'])
    expect(stored).toEqual([{ authorization: null, headers: signed, body: 'png-bytes' }])
    expect(storageInits).toHaveLength(1)
    expect(storageInits[0]).toMatchObject({ method: 'PUT', credentials: 'omit', headers: signed })
  })

  it('counts a 200 confirm as success', async () => {
    serveUpload({ confirm: () => HttpResponse.json(photo, { status: 200 }) })
    await expect(uploadPhoto(client, input)).resolves.toEqual(photo)
  })

  it('confirms a kit photo on the kit', async () => {
    const confirmed: unknown[] = []
    server.use(
      http.post(`${base}${admin}/photo-uploads`, uploadResponse),
      http.put(`${storage}/bucket/*`, () => new HttpResponse(null, { status: 200 })),
      http.post(`${base}${admin}/kits/${kitId}/photos`, async ({ request }) => {
        confirmed.push(await request.json())
        return HttpResponse.json(photo, { status: 201 })
      }),
    )

    await expect(uploadPhoto(client, { ownerType: 'kit', ownerId: kitId, file })).resolves.toEqual(photo)
    expect(confirmed).toEqual([{ key }])
  })

  it.each(['photo_invalid', 'photo_not_uploaded', 'photo_limit'])('surfaces %s from the confirm', async (code) => {
    serveUpload({ confirm: () => problemResponse(code === 'photo_limit' ? 409 : 400, code) })
    expect(await codeOf(uploadPhoto(client, input))).toBe(code)
  })

  it('throws PhotoUploadError and does not confirm when storage refuses the PUT', async () => {
    const { calls } = serveUpload({ storageStatus: 403 })

    const failure = await uploadPhoto(client, input).catch((error: unknown) => error)

    expect(failure).toBeInstanceOf(PhotoUploadError)
    expect(failure).toMatchObject({ code: 'photo_upload_failed', status: 403 })
    expect(calls).toEqual(['request', 'put'])
  })

  it('throws PhotoUploadError when storage cannot be reached', async () => {
    server.use(
      http.post(`${base}${admin}/photo-uploads`, uploadResponse),
      http.put(`${storage}/bucket/*`, () => HttpResponse.error()),
    )
    const failure = await uploadPhoto(client, input).catch((error: unknown) => error)
    expect(failure).toBeInstanceOf(PhotoUploadError)
    expect(failure).toMatchObject({ code: 'photo_upload_failed', status: 0 })
  })

  it('refuses a file type the API does not accept before any request', async () => {
    const { calls } = serveUpload({})
    const gif = new Blob(['x'], { type: 'image/gif' })

    const failure = await uploadPhoto(client, { ...input, file: gif }).catch((error: unknown) => error)

    expect(failure).toBeInstanceOf(PhotoFileError)
    expect(failure).toMatchObject({ code: 'photo_type_unsupported' })
    expect(calls).toEqual([])
  })

  it('refuses a file over 10 MB before any request', async () => {
    const { calls } = serveUpload({})
    const big = new Blob([new Uint8Array(10 * 1024 * 1024 + 1)], { type: 'image/jpeg' })

    const failure = await uploadPhoto(client, { ...input, file: big }).catch((error: unknown) => error)

    expect(failure).toBeInstanceOf(PhotoFileError)
    expect(failure).toMatchObject({ code: 'photo_too_large' })
    expect(calls).toEqual([])
  })

  it('rethrows the caller abort mid-PUT as is and does not confirm', async () => {
    const controller = new AbortController()
    const calls: string[] = []
    server.use(
      http.post(`${base}${admin}/photo-uploads`, uploadResponse),
      http.put(`${storage}/bucket/*`, async () => {
        calls.push('put')
        controller.abort(new DOMException('closed', 'AbortError'))
        await delay(200)
        return new HttpResponse(null, { status: 200 })
      }),
      http.post(`${base}${admin}/items/${itemId}/photos`, () => {
        calls.push('confirm')
        return HttpResponse.json(photo, { status: 201 })
      }),
    )

    const failure = await uploadPhoto(client, { ...input, signal: controller.signal }).catch((error: unknown) => error)

    expect(failure).not.toBeInstanceOf(PhotoUploadError)
    expect(failure).toMatchObject({ name: 'AbortError' })
    expect(calls).toEqual(['put'])
  })

  it('turns a storage timeout into PhotoUploadError and does not confirm', async () => {
    const calls: string[] = []
    server.use(
      http.post(`${base}${admin}/photo-uploads`, uploadResponse),
      http.put(`${storage}/bucket/*`, async () => {
        calls.push('put')
        await delay(500)
        return new HttpResponse(null, { status: 200 })
      }),
      http.post(`${base}${admin}/items/${itemId}/photos`, () => {
        calls.push('confirm')
        return HttpResponse.json(photo, { status: 201 })
      }),
    )

    const failure = await uploadPhoto(client, { ...input, storageTimeoutMs: 20 }).catch((error: unknown) => error)

    expect(failure).toBeInstanceOf(PhotoUploadError)
    expect(failure).toMatchObject({ status: 0 })
    expect(calls).toEqual(['put'])
  })

  it('still times out the PUT when the caller also passes a signal', async () => {
    server.use(
      http.post(`${base}${admin}/photo-uploads`, uploadResponse),
      http.put(`${storage}/bucket/*`, async () => {
        await delay(500)
        return new HttpResponse(null, { status: 200 })
      }),
    )
    const controller = new AbortController()

    const failure = await uploadPhoto(client, { ...input, signal: controller.signal, storageTimeoutMs: 20 }).catch((error: unknown) => error)

    expect(failure).toBeInstanceOf(PhotoUploadError)
  })

  it('surfaces photo_limit from the upload request and never calls storage', async () => {
    problem(409, 'photo_limit')
    expect(await codeOf(uploadPhoto(client, input))).toBe('photo_limit')
  })
})
