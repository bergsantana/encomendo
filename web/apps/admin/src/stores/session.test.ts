import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it } from 'vitest'

import { useSessionStore } from './session'

beforeEach(() => setActivePinia(createPinia()))

const grant = { accessToken: 'token-of-the-owner', permissions: ['order:read', 'order:write'] }

describe('useSessionStore', () => {
  it('starts signed out, with no token and no permissions', () => {
    const session = useSessionStore()

    expect(session.isSignedIn).toBe(false)
    expect(session.accessToken()).toBeNull()
    expect(session.can('order:read')).toBe(false)
  })

  it('holds the token and permissions of a started session', () => {
    const session = useSessionStore()

    session.start(grant)

    expect(session.isSignedIn).toBe(true)
    expect(session.accessToken()).toBe('token-of-the-owner')
    expect(session.can('order:write')).toBe(true)
    expect(session.can('finance:read')).toBe(false)
  })

  it('replaces the permissions when a new grant arrives', () => {
    const session = useSessionStore()
    session.start(grant)

    session.start({ accessToken: 'token-in-another-store', permissions: ['order:read'] })

    expect(session.accessToken()).toBe('token-in-another-store')
    expect(session.can('order:write')).toBe(false)
  })

  it('forgets everything when the session ends', () => {
    const session = useSessionStore()
    session.start(grant)

    session.end()

    expect(session.isSignedIn).toBe(false)
    expect(session.accessToken()).toBeNull()
    expect(session.can('order:read')).toBe(false)
  })

  it('keeps the access token out of the store state and out of web storage', () => {
    const session = useSessionStore()

    session.start(grant)

    // Store state is what devtools show and persistence plugins write to
    // localStorage, so the token must never be part of it.
    expect(JSON.stringify(session.$state)).not.toContain('token-of-the-owner')
    expect(localStorage.length).toBe(0)
    expect(sessionStorage.length).toBe(0)
  })
})
