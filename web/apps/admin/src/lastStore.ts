const key = 'encomendo.lastStore'
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i

// The store id is not a secret, and losing it only loses a convenience, so
// blocked or missing storage (private windows, strict settings) is ignored.

/**
 * rememberStore keeps the id of the store last used on this device, so the
 * next login opens it (`LoginRequest.tenantId`).
 */
export function rememberStore(tenantId: string): void {
  try {
    localStorage.setItem(key, tenantId)
  } catch {
    // Storage blocked: the next login shows the store picker instead.
  }
}

/**
 * lastStore returns the store last used on this device, if the browser kept
 * it. Anything but a UUID is ignored: sent as `tenantId`, it would make every
 * login on this device fail as invalid.
 */
export function lastStore(): string | undefined {
  try {
    const stored = localStorage.getItem(key)
    return stored !== null && uuid.test(stored) ? stored : undefined
  } catch {
    return undefined
  }
}
