import { createHttpClient } from '@encomendo/api'

/**
 * authClient makes the /auth calls. It has no session hooks: those calls
 * authenticate with a password or the refresh cookie (ADR 0010), and the
 * session store refreshes through it, so it must never depend on the
 * session itself. Same-origin, like `api`.
 */
export const authClient = createHttpClient({ baseUrl: window.location.origin })
