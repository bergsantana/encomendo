import { defineStore } from 'pinia'
import { computed, ref, shallowRef } from 'vue'

/**
 * Permission is a name such as "order:write". The backend owns the role →
 * permission map (docs/architecture.md, section 5.4) and sends the list with
 * the session, so the web app never keeps its own copy.
 */
export type Permission = string

/** SessionGrant is what a login, refresh or tenant switch gives the web app. */
export interface SessionGrant {
  accessToken: string
  permissions: readonly Permission[]
}

/**
 * useSessionStore holds the signed-in member's access token and permissions.
 * The token lives in memory only; after a reload the app gets a new one from
 * the httpOnly refresh cookie, which JavaScript never reads.
 */
export const useSessionStore = defineStore('session', () => {
  // Deliberately not returned: returned refs become store state, which
  // devtools show and persistence plugins write to localStorage. Keeping the
  // token in the closure means it can only ever be in memory.
  const token = shallowRef<string | null>(null)
  const permissions = ref<ReadonlySet<Permission>>(new Set())

  const isSignedIn = computed(() => token.value !== null)

  /** start begins a session, or replaces it after a refresh or tenant switch. */
  function start(grant: SessionGrant): void {
    token.value = grant.accessToken
    permissions.value = new Set(grant.permissions)
  }

  /** end forgets the token and permissions. It does not call the API. */
  function end(): void {
    token.value = null
    permissions.value = new Set()
  }

  /** accessToken returns the token for the HTTP client, or null when signed out. */
  function accessToken(): string | null {
    return token.value
  }

  /**
   * can reports whether the session holds a permission. It only hides UI:
   * the backend checks every call again.
   */
  function can(permission: Permission): boolean {
    return permissions.value.has(permission)
  }

  return { permissions, isSignedIn, start, end, accessToken, can }
})
