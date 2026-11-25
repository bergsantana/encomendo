import type { Permission } from '@encomendo/api'
import type { NavigationGuard } from 'vue-router'

import { useSessionStore } from '../stores/session'
import { pathWithoutFragment, safeRedirect } from './redirect'

declare module 'vue-router' {
  interface RouteMeta {
    /** requiresAuth sends signed-out users to login. Child routes inherit it. */
    requiresAuth?: boolean
    /**
     * permission hides the screen from roles that lack it. It implies
     * requiresAuth, and it only hides UI: the API checks the permission again.
     */
    permission?: Permission
    /** guestOnly sends a signed-in member to the home screen, as login does. */
    guestOnly?: boolean
  }
}

/** AuthGuardRoutes names the routes the guard redirects to. */
export interface AuthGuardRoutes {
  /**
   * login receives the original path, without its fragment, in the
   * `redirect` query. It must be a public top-level route: under a
   * `requiresAuth` parent it would redirect to itself forever.
   */
  login: string
  /**
   * home is where a member lands without the permission, and where a
   * signed-in member opening login lands when there is no `redirect`. It must
   * not need a permission itself.
   */
  home: string
}

/**
 * createAuthGuard returns a navigation guard that first restores the session
 * from the refresh cookie (once per page load, so a reload of a deep link
 * keeps the page), then checks `guestOnly`, `requiresAuth` and `permission`
 * on every matched route record, not on the merged `to.meta`, so a child can
 * never drop its parent's requirement.
 */
export function createAuthGuard(routes: AuthGuardRoutes): NavigationGuard {
  return async (to) => {
    const session = useSessionStore()
    await session.restore()

    if (to.matched.some((record) => record.meta.guestOnly === true)) {
      return session.isSignedIn ? (safeRedirect(to.query.redirect) ?? { name: routes.home }) : true
    }

    const permissions = to.matched.flatMap((record) => record.meta.permission ?? [])
    const needsAuth = permissions.length > 0 || to.matched.some((record) => record.meta.requiresAuth === true)

    if (needsAuth && !session.isSignedIn) return { name: routes.login, query: { redirect: pathWithoutFragment(to.fullPath) } }
    if (!permissions.every((permission) => session.can(permission))) return { name: routes.home }
    return true
  }
}
