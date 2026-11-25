/** The BroadcastChannel tabs use to say whose session the shared refresh cookie now holds. */
const channelName = 'encomendo-session'

/**
 * CookieOwner is whose session the shared refresh cookie holds after a
 * login, sign-up, store switch or logout; null ids mean no session. It never
 * carries a token.
 */
export interface CookieOwner {
  userId: string | null
  tenantId: string | null
}

/** SessionChannel announces this tab's changes to the other tabs of the app. */
export interface SessionChannel {
  announce(owner: CookieOwner): void
  close(): void
}

/**
 * openSessionChannel connects this tab to the others. Without it a tab would
 * only notice a logout or switch elsewhere at its next refresh, up to 15
 * minutes later, still showing the old session meanwhile. Browsers without
 * BroadcastChannel get a channel that does nothing.
 */
export function openSessionChannel(onAnnounce: (owner: CookieOwner) => void): SessionChannel {
  if (!('BroadcastChannel' in globalThis)) return { announce: () => undefined, close: () => undefined }

  const channel = new BroadcastChannel(channelName)
  channel.onmessage = (event: MessageEvent<unknown>) => {
    if (isCookieOwner(event.data)) onAnnounce(event.data)
  }
  return {
    announce: (owner) => channel.postMessage(owner),
    close: () => channel.close(),
  }
}

function isCookieOwner(value: unknown): value is CookieOwner {
  if (typeof value !== 'object' || value === null) return false
  const { userId, tenantId } = value as Record<string, unknown>
  return (typeof userId === 'string' || userId === null) && (typeof tenantId === 'string' || tenantId === null)
}
