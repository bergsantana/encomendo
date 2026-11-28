import { afterEach, describe, expect, it, vi } from 'vitest'

import { idempotencyHeaders, newIdempotencyKey } from './idempotency'

const uuidV4 = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/

afterEach(() => vi.unstubAllGlobals())

describe('newIdempotencyKey', () => {
  it('uses crypto.randomUUID where there is one', () => {
    const randomUUID = vi.fn(() => '11111111-2222-4333-8444-555555555555')
    vi.stubGlobal('crypto', { randomUUID, getRandomValues: crypto.getRandomValues.bind(crypto) })

    expect(newIdempotencyKey()).toBe('11111111-2222-4333-8444-555555555555')
    expect(randomUUID).toHaveBeenCalledOnce()
  })

  it('makes a version 4 UUID from getRandomValues where crypto.randomUUID is missing, as on a plain-HTTP dev URL', () => {
    // No randomUUID, as in an insecure context.
    vi.stubGlobal('crypto', { getRandomValues: crypto.getRandomValues.bind(crypto) })

    const keys = new Set(Array.from({ length: 50 }, () => newIdempotencyKey()))

    expect(keys.size).toBe(50)
    for (const key of keys) expect(key).toMatch(uuidV4)
  })

  it('sets the version and variant bits whatever the random bytes are', () => {
    vi.stubGlobal('crypto', {
      getRandomValues: (bytes: Uint8Array) => {
        bytes.fill(0xff)
        return bytes
      },
    })

    expect(newIdempotencyKey()).toBe('ffffffff-ffff-4fff-bfff-ffffffffffff')
  })

  it('is a header the API reads', () => {
    expect(idempotencyHeaders('abc')).toEqual({ 'Idempotency-Key': 'abc' })
  })
})
