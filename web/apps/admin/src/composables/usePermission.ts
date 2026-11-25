import { computed, type ComputedRef } from 'vue'

import { type Permission, useSessionStore } from '../stores/session'

/**
 * usePermission reports, reactively, whether the signed-in member may use an
 * action, so a view can hide what the role cannot do. It never decides
 * access: the backend checks the same permission on every call.
 */
export function usePermission(permission: Permission): ComputedRef<boolean> {
  const session = useSessionStore()
  return computed(() => session.can(permission))
}
