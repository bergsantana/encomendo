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

  constructor(status: number, body: ProblemBody) {
    super(body.title ?? `HTTP ${status}`)
    this.name = 'ApiError'
    this.status = status
    this.code = body.code ?? (status === 0 ? 'network' : 'unknown')
    this.title = body.title ?? 'Erro inesperado'
    this.detail = body.detail
    this.fieldIssues = body.errors ?? []
  }
}

export interface HttpClientOptions {
  /** Prefix for every path, e.g. "" for same-origin or "https://api.encomendo.com.br". */
  baseUrl: string
  /** Injected for tests; defaults to the global fetch. */
  fetch?: typeof fetch
}

export interface HttpClient {
  get<T>(path: string, init?: RequestInit): Promise<T>
  post<T>(path: string, body: unknown, init?: RequestInit): Promise<T>
}

/** createHttpClient returns a JSON client that turns problem+json into ApiError. */
export function createHttpClient(options: HttpClientOptions): HttpClient {
  // Looked up on every call, not captured once, so test mocks and polyfills
  // installed after the client is created still apply.
  const doFetch: typeof fetch = options.fetch ?? ((input, init) => globalThis.fetch(input, init))

  async function request<T>(method: string, path: string, body: unknown, init: RequestInit = {}): Promise<T> {
    const headers = new Headers(init.headers)
    headers.set('Accept', 'application/json, application/problem+json')
    if (body !== undefined) headers.set('Content-Type', 'application/json')

    let response: Response
    try {
      response = await doFetch(options.baseUrl + path, {
        ...init,
        method,
        headers,
        credentials: 'include', // the refresh cookie is httpOnly
        body: body === undefined ? null : JSON.stringify(body),
      })
    } catch {
      throw new ApiError(0, { title: 'Sem conexão com o servidor' })
    }

    if (!response.ok) throw new ApiError(response.status, await readProblem(response))
    if (response.status === 204) return undefined as T
    return (await response.json()) as T
  }

  return {
    get: (path, init) => request('GET', path, undefined, init),
    post: (path, body, init) => request('POST', path, body, init),
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
