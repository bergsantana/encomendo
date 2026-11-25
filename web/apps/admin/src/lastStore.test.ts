import { afterEach, describe, expect, it, vi } from 'vitest'

import { lastStore, rememberStore } from './lastStore'

afterEach(() => {
  localStorage.clear()
  vi.restoreAllMocks()
})

describe('lastStore', () => {
  it('remembers the store last used on this device', () => {
    rememberStore('0190f3c4-5e1a-7b2c-8d3e-4f5a6b7c8d00')

    expect(lastStore()).toBe('0190f3c4-5e1a-7b2c-8d3e-4f5a6b7c8d00')
  })

  it('has nothing to offer on a new device', () => {
    expect(lastStore()).toBeUndefined()
  })

  it.each(['not-a-uuid', '', '0190f3c4-5e1a-7b2c-8d3e-4f5a6b7c8d00 OR 1=1'])('ignores a stored value that is not a store id: %j', (value) => {
    // Sent as is, it would make every login on this device fail as invalid.
    localStorage.setItem('encomendo.lastStore', value)

    expect(lastStore()).toBeUndefined()
  })

  it('carries on without the hint when the browser blocks storage', () => {
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new DOMException('blocked', 'SecurityError')
    })
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new DOMException('blocked', 'SecurityError')
    })

    expect(() => rememberStore('0190f3c4-5e1a-7b2c-8d3e-4f5a6b7c8d00')).not.toThrow()
    expect(lastStore()).toBeUndefined()
  })
})
