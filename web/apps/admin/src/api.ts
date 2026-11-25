import { createHttpClient } from '@encomendo/api'

/**
 * api is the admin app's HTTP client. It is same-origin: nginx and the dev
 * proxy route /api to the backend, so no CORS and the refresh cookie just works.
 */
export const api = createHttpClient({ baseUrl: window.location.origin })
