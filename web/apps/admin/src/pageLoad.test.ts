import { describe, expect, it, vi } from 'vitest'

import { reloadWhenRestoredFromCache } from './pageLoad'

/** A window as far as the back/forward cache is concerned. */
function fakeWindow() {
  const reload = vi.fn()
  const host = Object.assign(new EventTarget(), { location: { reload } })
  reloadWhenRestoredFromCache(host)
  const show = (persisted: boolean) => host.dispatchEvent(Object.assign(new Event('pageshow'), { persisted }))
  return { reload, show }
}

describe('reloadWhenRestoredFromCache', () => {
  it('reloads a page the browser brought back from its back/forward cache', () => {
    // After a logout, Back would otherwise show the previous member's screens as they were.
    const { reload, show } = fakeWindow()

    show(true)

    expect(reload).toHaveBeenCalledOnce()
  })

  it('leaves a normal page load alone', () => {
    const { reload, show } = fakeWindow()

    show(false)

    expect(reload).not.toHaveBeenCalled()
  })
})
