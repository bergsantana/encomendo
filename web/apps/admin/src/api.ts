import { createHttpClient } from '@encomendo/api'

import { useSessionStore } from './stores/session'

/**
 * api is the admin app's HTTP client for /api routes. It is same-origin:
 * nginx and the dev proxy route /api to the backend, so there is no CORS and
 * the refresh cookie just works. It sends the session's access token and
 * refreshes it once on a 401. A `tenant_suspended` answer to any call ends the
 * session in this page, which then opens the store picker (ADR 0022).
 */
export const api = createHttpClient({
  baseUrl: window.location.origin,
  // The store is looked up per call: it needs the app's Pinia, which does
  // not exist yet when this module loads.
  session: {
    accessToken: () => useSessionStore().accessToken(),
    refresh: () => useSessionStore().refresh(),
    scope: () => useSessionStore().scope(),
  },
  onProblem: (error) => {
    if (error.code === 'tenant_suspended') useSessionStore().storeSuspended()
  },
})
