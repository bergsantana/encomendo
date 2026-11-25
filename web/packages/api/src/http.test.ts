import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, describe, expect, it } from 'vitest'

import { getReadiness } from './health'
import { ApiError, createHttpClient } from './http'

const base = 'http://api.test'
const server = setupServer()

beforeAll(() => server.listen({ onUnhandledFrame: 'error' }))
afterEach(() => server.resetHandlers())
afterAll(() => server.close())

const client = createHttpClient({ baseUrl: base })

describe('createHttpClient', () => {
  it('returns the parsed JSON body', async () => {
    server.use(http.get(`${base}/readyz`, () => HttpResponse.json({ status: 'ok', checks: { postgres: 'ok' } })))

    await expect(getReadiness(client)).resolves.toEqual({ status: 'ok', checks: { postgres: 'ok' } })
  })

  it('sends JSON bodies with the right headers', async () => {
    let seen: { type: string | null; body: unknown } | undefined
    server.use(
      http.post(`${base}/echo`, async ({ request }) => {
        seen = { type: request.headers.get('Content-Type'), body: await request.json() }
        return new HttpResponse(null, { status: 204 })
      }),
    )

    await expect(client.post('/echo', { qty: 2 })).resolves.toBeUndefined()
    expect(seen).toEqual({ type: 'application/json', body: { qty: 2 } })
  })

  it('turns problem+json into an ApiError with code and field issues', async () => {
    server.use(
      http.post(`${base}/orders`, () =>
        HttpResponse.json(
          {
            type: 'about:blank',
            title: 'Dados inválidos',
            status: 400,
            code: 'invalid',
            errors: [{ field: 'phone', message: 'Informe um WhatsApp válido.' }],
          },
          { status: 400, headers: { 'Content-Type': 'application/problem+json' } },
        ),
      ),
    )

    const error = await client.post('/orders', {}).catch((e: unknown) => e)
    expect(error).toBeInstanceOf(ApiError)
    expect(error).toMatchObject({ status: 400, code: 'invalid', title: 'Dados inválidos' })
    expect((error as ApiError).fieldIssues).toEqual([{ field: 'phone', message: 'Informe um WhatsApp válido.' }])
  })

  it('handles non-JSON error bodies', async () => {
    server.use(http.get(`${base}/broken`, () => new HttpResponse('Bad Gateway', { status: 502 })))

    await expect(client.get('/broken')).rejects.toMatchObject({ status: 502, code: 'unknown' })
  })

  it('reports network failures as status 0', async () => {
    server.use(http.get(`${base}/readyz`, () => HttpResponse.error()))

    await expect(getReadiness(client)).rejects.toMatchObject({ status: 0, code: 'network' })
  })
})
