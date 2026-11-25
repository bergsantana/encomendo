/**
 * The only code in the web workspace that talks HTTP. Stores call the typed
 * functions built on top of this client; components and views never do.
 */

/** One invalid field from a 400 problem response. */
export interface FieldIssue {
  field: string
  /** pt-BR text that can be shown next to the field. */
  message: string
}

/** Fields of an RFC 9457 problem+json body. */
export interface ProblemBody {
  type?: string
  title?: string
  status?: number
  code?: string
  detail?: string
  instance?: string
  errors?: FieldIssue[]
}

/**
 * ApiError is thrown for every non-2xx response and for network failures.
 * Branch on `code`, never on `title`: codes are stable, titles are wording.
 */
export class ApiError extends Error {
  readonly status: number
  readonly code: string
  readonly title: string
  readonly detail: string | undefined
  readonly fieldIssues: FieldIssue[]
  /** retryAfterSeconds is the wait a 429 asks for, from its Retry-After header. */
  readonly retryAfterSeconds: number | undefined

  constructor(status: number, body: ProblemBody, retryAfterSeconds?: number) {
    super(body.title ?? `HTTP ${status}`)
    this.name = 'ApiError'
    this.status = status
    this.code = body.code ?? (status === 0 ? 'network' : 'unknown')
    this.title = body.title ?? 'Erro inesperado'
    this.detail = body.detail
    this.fieldIssues = body.errors ?? []
    this.retryAfterSeconds = retryAfterSeconds
  }
}

/**
 * SessionHooks connect the client to the app's session. The client never
 * stores a token itself: it asks for the current one on every request, so a
 * refresh or a tenant switch applies to the next call without rebuilding it.
 */
export interface SessionHooks {
  /** accessToken returns the bearer token to send, or null when signed out. */
  accessToken(): string | null
  /**
   * refresh trades the refresh cookie for a new access token and resolves
   * whether there is a token to retry with: false once the session is over,
   * e.g. when /auth/refresh refuses with 400, 401 or 403. Offline, 429 and
   * 5xx must reject with their own error and keep the session (ADR 0012).
   * Concurrent calls must share one refresh: the refresh token rotates on
   * every use, and two refreshes in flight revoke the whole session family
   * (ADR 0010, ADR 0013).
   */
  refresh(): Promise<boolean>
  /**
   * scope names the session a call is made for, such as the user and the
   * store, or null when signed out. A call is retried only under the scope it
   * was sent with, so a write meant for one store never runs in another
   * after a switch, nor as another user after a login elsewhere (ADR 0012).
   */
  scope(): string | null
}

/** HttpClientOptions configure one client; the admin and store apps each build their own. */
export interface HttpClientOptions {
  /** Prefix for every path, e.g. "" for same-origin or "https://api.encomendo.com.br". */
  baseUrl: string
  /** Injected for tests; defaults to the global fetch. */
  fetch?: typeof fetch
  /**
   * session makes the client send the access token and refresh it once on a
   * 401. Leave it out for a client that only calls public routes; otherwise
   * mark the individual calls that must not carry the token as `anonymous`.
   */
  session?: SessionHooks
  /** timeoutMs is how long a call may take before it fails like a lost connection. Defaults to 30 s. */
  timeoutMs?: number
}

// The same timeout as Android. ADR 0013 sizes its 2-minute refresh retry
// window on it, and without it a hung refresh would hold the cross-tab lock.
const defaultTimeoutMs = 30_000

/** RequestOptions are fetch options plus the client's own flags. */
export interface RequestOptions extends Omit<RequestInit, 'method' | 'body'> {
  /**
   * anonymous sends no bearer token and never refreshes on a 401. Use it for
   * every /auth call: sign-up and login authenticate with a password, and
   * refresh, switch-tenant and logout with the refresh cookie (ADR 0010).
   * A 401 from those means a wrong password or a dead refresh token, so
   * refreshing would loop or present a rotated token. Logout may still pass
   * the access token in `headers` so the API revokes it at once.
   */
  anonymous?: boolean
}

/**
 * HttpClient is what every packages/api function takes. Each method resolves
 * with the parsed JSON body (undefined for a bodiless 202 or 204) and throws ApiError for any
 * non-2xx response or network failure.
 */
export interface HttpClient {
  get<T>(path: string, options?: RequestOptions): Promise<T>
  post<T>(path: string, body: unknown, options?: RequestOptions): Promise<T>
  put<T>(path: string, body: unknown, options?: RequestOptions): Promise<T>
  delete<T>(path: string, options?: RequestOptions): Promise<T>
}

/** Call is one request as the caller asked for it; a retry sends it again unchanged. */
interface Call {
  method: string
  path: string
  body: unknown
  init: RequestInit
}

/**
 * createHttpClient returns a JSON client that turns problem+json into
 * ApiError. With session hooks it sends the access token and, on a 401,
 * renews it once and retries once; a second 401 is thrown to the caller.
 */
export function createHttpClient(options: HttpClientOptions): HttpClient {
  // Looked up on every call, not captured once, so test mocks and polyfills
  // installed after the client is created still apply.
  const doFetch: typeof fetch = options.fetch ?? ((input, init) => globalThis.fetch(input, init))
  const timeoutMs = options.timeoutMs ?? defaultTimeoutMs

  async function send(call: Call, token: string | null): Promise<Response> {
    const headers = new Headers(call.init.headers)
    headers.set('Accept', 'application/json, application/problem+json')
    if (call.body !== undefined) headers.set('Content-Type', 'application/json')
    if (token !== null) headers.set('Authorization', `Bearer ${token}`)

    try {
      return await doFetch(options.baseUrl + call.path, {
        ...call.init,
        method: call.method,
        headers,
        credentials: 'include', // the refresh cookie is httpOnly
        body: call.body === undefined ? null : JSON.stringify(call.body),
        signal: withTimeout(call.init.signal, timeoutMs),
      })
    } catch {
      throw new ApiError(0, { title: 'Sem conexão com o servidor' })
    }
  }

  async function request<T>(method: string, path: string, body: unknown, requestOptions: RequestOptions = {}): Promise<T> {
    assertPath(path)
    const { anonymous = false, ...init } = requestOptions
    const call: Call = { method, path, body, init }
    const session = anonymous ? undefined : options.session

    const sent = session?.accessToken() ?? null
    const sentFor = session?.scope() ?? null
    let response = await send(call, sent)
    if (response.status === 401 && session !== undefined && (await renew(session, sent)) && session.scope() === sentFor) {
      response = await send(call, session.accessToken())
    }

    if (!response.ok) throw new ApiError(response.status, await readProblem(response), retryAfter(response))
    return readBody<T>(response)
  }

  return {
    get: (path, requestOptions) => request('GET', path, undefined, requestOptions),
    post: (path, body, requestOptions) => request('POST', path, body, requestOptions),
    put: (path, body, requestOptions) => request('PUT', path, body, requestOptions),
    delete: (path, requestOptions) => request('DELETE', path, undefined, requestOptions),
  }
}

/**
 * renew gets a token to retry with after a 401. When another request already
 * refreshed while this one was in flight, its new token is reused: refreshing
 * again would rotate the refresh cookie for nothing (ADR 0012).
 */
async function renew(session: SessionHooks, sent: string | null): Promise<boolean> {
  const current = session.accessToken()
  if (current !== null && current !== sent) return true
  return session.refresh()
}

/**
 * assertPath refuses a path that does not start with a single "/": glued to
 * the base URL, ".evil.example/x" or "//evil.example" would name another host
 * and hand it the bearer token.
 */
function assertPath(path: string): void {
  if (!path.startsWith('/') || path.startsWith('//')) throw new Error(`path must start with a single /: ${path}`)
}

/** withTimeout aborts the call after `ms`, or earlier if the caller's own signal aborts. */
function withTimeout(callerSignal: AbortSignal | null | undefined, ms: number): AbortSignal {
  const timeout = AbortSignal.timeout(ms)
  return callerSignal ? AbortSignal.any([callerSignal, timeout]) : timeout
}

/** retryAfter reads Retry-After as whole seconds, at least 1, the only form the API sends (ADR 0013). */
function retryAfter(response: Response): number | undefined {
  const seconds = Number(response.headers.get('Retry-After') ?? Number.NaN)
  return Number.isInteger(seconds) && seconds >= 1 ? seconds : undefined
}

/**
 * readBody parses a success body. Only 202 and 204, the answers the contract
 * gives without a body, may be empty, and resolve undefined. Anything else
 * that is empty, stops arriving (the timeout also covers it) or is not JSON,
 * such as a web page from a server without the API, is an answer the app
 * cannot use: it fails like a lost connection, so callers only ever see
 * ApiError and never an undefined session.
 */
async function readBody<T>(response: Response): Promise<T> {
  try {
    const text = await response.text()
    if (text === '' && (response.status === 202 || response.status === 204)) return undefined as T
    return JSON.parse(text) as T
  } catch {
    throw new ApiError(0, { title: 'Sem conexão com o servidor' })
  }
}

async function readProblem(response: Response): Promise<ProblemBody> {
  const type = response.headers.get('Content-Type') ?? ''
  if (!type.includes('json')) return { status: response.status }
  try {
    return (await response.json()) as ProblemBody
  } catch {
    return { status: response.status }
  }
}
