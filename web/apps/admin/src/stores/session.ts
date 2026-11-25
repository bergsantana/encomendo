import {
  ApiError,
  type AuthSession,
  type HttpClient,
  logOut as revokeSession,
  type Permission,
  refreshSession,
  type Session,
  switchTenant,
} from '@encomendo/api'
import { defineStore } from 'pinia'
import { computed, onScopeDispose, ref, shallowRef } from 'vue'

import { authClient } from '../authClient'
import { rememberStore } from '../lastStore'
import { type CookieOwner, openSessionChannel } from './sessionChannel'

/** The Web Lock every tab holds around calls that set the refresh cookie, so tabs take turns (ADR 0010). */
const refreshLockName = 'encomendo-refresh'

/**
 * SessionEnd says why the session ended: the member logged out, the server
 * refused the refresh, or the refresh came back for another user or store.
 * That last one happens when another tab logged in or switched store (the
 * refresh cookie is shared), and when the member was removed from the store
 * (their refresh returns a session without it, ADR 0010). `switched` means
 * this tab moved to another store and is about to load it with a full page
 * load; until then nothing may run in this page under either store.
 */
export type SessionEnd = 'logged_out' | 'expired' | 'replaced' | 'switched'

/**
 * useSessionStore holds the signed-in member's session: the access token,
 * the user, the selected store, the role and permissions there, and every
 * membership. The access token lives in memory only; after a reload,
 * `restore` gets a new one from the httpOnly refresh cookie.
 */
export const useSessionStore = defineStore('session', () => {
  // Deliberately not returned: returned refs become store state, which
  // devtools show and persistence plugins write to localStorage. Keeping the
  // token in the closure means it can only ever be in memory.
  const token = shallowRef<string | null>(null)
  const current = shallowRef<Session | null>(null)
  /** ended is why the last session ended, or null while one is running or none has started. */
  const ended = ref<SessionEnd | null>(null)
  /** restoreError is why the server could not restore the session on load, while that is still the case. */
  const restoreError = shallowRef<ApiError | null>(null)

  // Bumped by every start and end, so a refresh can tell whether the session
  // it began from is still the one in place (ADR 0012).
  let generation = 0
  let refreshing: Promise<boolean> | null = null
  let restoring: Promise<void> | null = null
  const takeTurn = turnTaker()
  const otherTabs = openSessionChannel(onOtherTab)
  onScopeDispose(() => otherTabs.close())

  const isSignedIn = computed(() => token.value !== null)
  const user = computed(() => current.value?.user ?? null)
  const tenant = computed(() => current.value?.tenant ?? null)
  const role = computed(() => current.value?.role ?? null)
  const memberships = computed(() => current.value?.memberships ?? [])
  const granted = computed(() => new Set<Permission>(current.value?.permissions ?? []))

  /** start begins a session, or replaces it after a refresh. Calls that log in go through `begin`. */
  function start(auth: AuthSession): void {
    generation += 1
    token.value = auth.accessToken
    // Copied field by field, so no token can ever reach store state.
    current.value = {
      user: auth.user,
      tenant: auth.tenant,
      role: auth.role,
      permissions: auth.permissions,
      memberships: auth.memberships,
    }
    ended.value = null
    restoreError.value = null
    if (auth.tenant !== undefined) rememberStore(auth.tenant.id)
  }

  /** end forgets the session in this tab and records why. It does not call the API; logOut does. */
  function end(reason: SessionEnd): void {
    generation += 1
    token.value = null
    current.value = null
    ended.value = reason
  }

  /** accessToken returns the token for the HTTP client, or null when signed out. */
  function accessToken(): string | null {
    return token.value
  }

  /** scope names whom calls are made for, user and store, so the HTTP client never retries one under another (ADR 0012). */
  function scope(): string | null {
    const session = current.value
    return session === null ? null : `${session.user.id}@${session.tenant?.id ?? ''}`
  }

  /** can reports whether the role holds a permission. It only hides UI: the backend checks every call. */
  function can(permission: Permission): boolean {
    return granted.value.has(permission)
  }

  /**
   * begin runs a call that sets the refresh cookie (login, sign-up, store
   * switch, and invitations later) in its turn with every other such call,
   * so no refresh answer can land after it and overwrite its cookie, then
   * starts the session it returns and tells the other tabs. The call gets the
   * /auth client and must use only that: a call through `api` that met a 401
   * would wait for a refresh, and the refresh would wait for this very turn.
   */
  async function begin(call: (client: HttpClient) => Promise<AuthSession>): Promise<void> {
    const auth = await takeTurn(() => call(authClient))
    start(auth)
    otherTabs.announce({ userId: auth.user.id, tenantId: auth.tenant?.id ?? null })
  }

  /**
   * switchTo moves the session to another store the member belongs to, in
   * its turn like `begin`. Once the server has switched, this tab's session
   * ends as `switched`: the caller loads the new store with a full page load,
   * and until then nothing may run in this page under either store. A
   * refusal refreshes once, so a store the member lost leaves the list (403)
   * and a dead session ends (401; the contract says refresh, then log in).
   */
  async function switchTo(tenantId: string): Promise<void> {
    let auth: AuthSession
    try {
      auth = await takeTurn(() => switchTenant(authClient, tenantId))
    } catch (error) {
      // After the turn, so the refresh can take its own.
      if (error instanceof ApiError && (error.status === 401 || error.status === 403)) await refresh().catch(() => undefined)
      throw error
    }
    end('switched')
    if (auth.tenant !== undefined) rememberStore(auth.tenant.id)
    otherTabs.announce({ userId: auth.user.id, tenantId: auth.tenant?.id ?? null })
  }

  /**
   * onOtherTab ends this tab's session when another tab says the shared
   * cookie now holds a different session (or none), so this tab stops
   * showing and acting on a session that is no longer the browser's.
   */
  function onOtherTab(owner: CookieOwner): void {
    const mine = current.value
    if (mine === null) return
    if (owner.userId === mine.user.id && owner.tenantId === (mine.tenant?.id ?? null)) return
    end(owner.userId === null ? 'logged_out' : 'replaced')
  }

  /**
   * refresh trades the refresh cookie for a new session and resolves whether
   * the tab is signed in afterwards. Callers share one refresh in flight, and
   * it takes its turn with logins and logouts in every tab: two calls with
   * the same cookie would revoke the whole session family (ADR 0010, ADR
   * 0013). Only a refusal (400, 401, 403) ends the session; offline, 429 and
   * 5xx reject with their own error and keep it (ADR 0012).
   */
  function refresh(): Promise<boolean> {
    // Once this tab's session has ended, only a login or a page load starts
    // another. A late 401 must not refresh into whatever session the shared
    // cookie holds now and replay its request under it.
    if (ended.value !== null) return Promise.resolve(false)
    // The generation is read now, not once the turn comes, so a logout made
    // while waiting still wins.
    refreshing ??= renew(generation).finally(() => {
      refreshing = null
    })
    return refreshing
  }

  async function renew(startedFrom: number): Promise<boolean> {
    try {
      // A refresh whose turn comes after a login, logout or store switch is
      // not sent: the cookie in the jar belongs to that newer state now.
      const auth = await takeTurn(() => (generation === startedFrom ? refreshSession(authClient) : Promise.resolve(null)))
      // A login, logout or store switch made during the refresh wins (ADR 0012).
      if (auth === null || generation !== startedFrom) return isSignedIn.value
      if (changedHands(auth)) end('replaced')
      else start(auth)
    } catch (error) {
      if (!isRefusal(error)) throw error
      if (generation === startedFrom && isSignedIn.value) end('expired')
    }
    return isSignedIn.value
  }

  /**
   * changedHands reports a refreshed session for another user or store:
   * another tab logged in or switched, or the member lost this store.
   * Carrying on would mix this tab's screens with that session.
   */
  function changedHands(auth: AuthSession): boolean {
    const before = current.value
    if (before === null) return false
    return auth.user.id !== before.user.id || auth.tenant?.id !== before.tenant?.id
  }

  /**
   * restore signs the tab in from the refresh cookie on load. A refusal means
   * there is no session, and is final for this page load. Any other failure
   * leaves the tab signed out with `restoreError` set, and a later navigation
   * tries again: the cookie may still be good (ADR 0012).
   */
  function restore(): Promise<void> {
    if (isSignedIn.value) return Promise.resolve()
    restoring ??= refresh().then(
      () => {
        restoreError.value = null
      },
      (error: unknown) => {
        restoreError.value = error instanceof ApiError ? error : new ApiError(0, { title: 'Sem conexão com o servidor' })
        // Forgotten on the next task, not at once: the guard's own redirect to
        // login is part of the same navigation and must not ask again.
        setTimeout(() => {
          restoring = null
        }, 0)
      },
    )
    return restoring
  }

  /**
   * logOut revokes the session on the server, in its turn with refreshes so
   * it presents the newest cookie and access token, then forgets it here. If
   * the server cannot be reached the session stays and the error is thrown:
   * the refresh cookie is httpOnly, so only the server can end it, and
   * pretending otherwise would let the next person on this computer reload
   * into the session.
   */
  async function logOut(): Promise<void> {
    await takeTurn(() => revokeSession(authClient, token.value))
    end('logged_out')
    otherTabs.announce({ userId: null, tenantId: null })
  }

  return {
    current,
    ended,
    restoreError,
    isSignedIn,
    user,
    tenant,
    role,
    memberships,
    start,
    end,
    accessToken,
    can,
    scope,
    begin,
    switchTo,
    refresh,
    restore,
    logOut,
  }
})

function isRefusal(error: unknown): boolean {
  return error instanceof ApiError && [400, 401, 403].includes(error.status)
}

/**
 * turnTaker returns a function that runs calls one at a time: across tabs
 * through the Web Lock, and inside this tab through a queue. Without Web
 * Locks (old browsers, or a plain-HTTP origin other than localhost, where
 * browsers hide the API) tabs can still race, which can revoke the session;
 * this tab's own calls still wait their turn. Each call is bounded by the
 * HTTP client's timeout, so a turn never waits forever.
 */
function turnTaker(): <T>(task: () => Promise<T>) => Promise<T> {
  let queue: Promise<unknown> = Promise.resolve()
  return <T>(task: () => Promise<T>): Promise<T> => {
    const run = queue.then(() => ('locks' in navigator ? navigator.locks.request(refreshLockName, task) : task()))
    queue = run.catch(() => undefined)
    return run
  }
}
