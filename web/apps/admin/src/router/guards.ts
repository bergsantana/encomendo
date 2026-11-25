import type { NavigationGuard } from 'vue-router'

import { type Permission, useSessionStore } from '../stores/session'

declare module 'vue-router' {
  interface RouteMeta {
    /** requiresAuth sends signed-out users to login. Child routes inherit it. */
    requiresAuth?: boolean
    /**
     * permission hides the screen from roles that lack it. It implies
     * requiresAuth, and it only hides UI: the API checks the permission again.
     */
    permission?: Permission
  }
}

/** AuthGuardRoutes names the routes the guard redirects to. */
export interface AuthGuardRoutes {
  /**
   * login receives the original path in the `redirect` query. It must be a
   * public top-level route: under a `requiresAuth` parent it would redirect
   * to itself forever.
   */
  login: string
  /** home is where a member lands without the permission. It must not need one itself. */
  home: string
}

/**
 * createAuthGuard returns a navigation guard that checks `requiresAuth` and
 * `permission` on every matched route record, not on the merged `to.meta`,
 * so a child can never drop its parent's requirement. The guard reads the
 * session synchronously: register it only once the session has been restored
 * from the refresh cookie, or every reload of a deep link lands on login.
 */
export function createAuthGuard(routes: AuthGuardRoutes): NavigationGuard {
  return (to) => {
    const session = useSessionStore()
    const permissions = to.matched.flatMap((record) => record.meta.permission ?? [])
    const needsAuth = permissions.length > 0 || to.matched.some((record) => record.meta.requiresAuth === true)

    if (needsAuth && !session.isSignedIn) return { name: routes.login, query: { redirect: to.fullPath } }
    if (!permissions.every((permission) => session.can(permission))) return { name: routes.home }
    return true
  }
}
