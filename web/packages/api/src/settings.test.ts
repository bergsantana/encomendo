import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, describe, expect, it } from 'vitest'

import { ApiError, createHttpClient } from './http'
import { chooseStatusWorkflow, getSettings, updateSettings } from './settings'
import { getSettings as getSettingsFromTenancy } from './tenancy'
import { listStatusWorkflows } from './workflows'

const base = 'http://api.test'
const server = setupServer()

beforeAll(() => server.listen({ onUnhandledFrame: 'error' }))
afterEach(() => server.resetHandlers())
afterAll(() => server.close())

const client = createHttpClient({ baseUrl: base, session: { accessToken: () => 'access-1', refresh: async () => false, scope: () => 'ana@alta' } })

const workflowId = '0190f3c4-5e1a-7b2c-8d3e-4f5a6b7c8d01'
const settings = {
  stockEnabled: false,
  productionEnabled: false,
  purchaseBeforeDeposit: false,
  depositPercent: 50,
  assemblyDays: 3,
  safetyMarginDays: 2,
  whatsappPhone: null,
  timeZone: 'America/Sao_Paulo',
  currency: 'BRL',
  statusWorkflow: { id: workflowId, version: 1, name: 'Encomenda com montagem', statuses: [] },
  version: 4,
}

function problem(status: number, code: string): Response {
  return HttpResponse.json({ status, code, title: 'Problema' }, { status, headers: { 'Content-Type': 'application/problem+json' } })
}

describe('settings', () => {
  it('reads the settings, also through the older tenancy import', async () => {
    server.use(http.get(`${base}/api/v1/admin/settings`, () => HttpResponse.json(settings)))

    await expect(getSettings(client)).resolves.toEqual(settings)
    await expect(getSettingsFromTenancy(client)).resolves.toEqual(settings)
  })

  it('replaces them with every field, the time zone and the version read', async () => {
    let seen: { authorization: string | null; body: unknown } | undefined
    server.use(
      http.put(`${base}/api/v1/admin/settings`, async ({ request }) => {
        seen = { authorization: request.headers.get('Authorization'), body: await request.json() }
        return HttpResponse.json({ ...settings, version: 5 })
      }),
    )
    const body = {
      stockEnabled: true,
      productionEnabled: false,
      purchaseBeforeDeposit: true,
      depositPercent: 30,
      assemblyDays: 4,
      safetyMarginDays: 1,
      whatsappPhone: '5592991234567',
      timeZone: 'America/Manaus' as const,
      version: 4,
    }

    await expect(updateSettings(client, body)).resolves.toMatchObject({ version: 5 })
    expect(seen).toEqual({ authorization: 'Bearer access-1', body })
  })

  it('reports invalid fields, a stale version and a missing permission', async () => {
    server.use(
      http.put(`${base}/api/v1/admin/settings`, () =>
        HttpResponse.json(
          { status: 400, code: 'invalid', title: 'Dados inválidos', errors: [{ field: 'depositPercent', message: 'Use um sinal de 0 a 100%.' }] },
          { status: 400, headers: { 'Content-Type': 'application/problem+json' } },
        ),
      ),
    )
    const invalid = await updateSettings(client, { ...settings, version: 4, timeZone: 'America/Sao_Paulo' }).catch((error: unknown) => error)
    expect(invalid).toBeInstanceOf(ApiError)
    expect(invalid).toMatchObject({ status: 400, code: 'invalid', fieldIssues: [{ field: 'depositPercent', message: 'Use um sinal de 0 a 100%.' }] })

    server.use(http.put(`${base}/api/v1/admin/settings`, () => problem(409, 'version_conflict')))
    await expect(updateSettings(client, { ...settings, version: 3, timeZone: 'America/Sao_Paulo' })).rejects.toMatchObject({ code: 'version_conflict' })

    server.use(http.put(`${base}/api/v1/admin/settings`, () => problem(403, 'forbidden')))
    await expect(updateSettings(client, { ...settings, version: 4, timeZone: 'America/Sao_Paulo' })).rejects.toMatchObject({ status: 403, code: 'forbidden' })
  })
})

describe('status workflows', () => {
  it('lists the presets with their statuses', async () => {
    const list = {
      items: [
        {
          id: workflowId,
          version: 1,
          key: 'encomenda_montagem',
          name: 'Encomenda com montagem',
          statuses: [{ key: 'rascunho', label: 'Rascunho', clientLabel: 'Pedido recebido', phase: 'draft' }],
        },
      ],
    }
    server.use(http.get(`${base}/api/v1/admin/status-workflows`, () => HttpResponse.json(list)))

    await expect(listStatusWorkflows(client)).resolves.toEqual(list)
  })

  it('chooses one with the preset version and the settings version read', async () => {
    let body: unknown
    server.use(
      http.put(`${base}/api/v1/admin/settings/status-workflow`, async ({ request }) => {
        body = await request.json()
        return HttpResponse.json({ ...settings, version: 5 })
      }),
    )

    await expect(chooseStatusWorkflow(client, { workflowId, version: 2, expectedVersion: 4 })).resolves.toMatchObject({ version: 5 })
    expect(body).toEqual({ workflowId, version: 2, expectedVersion: 4 })
  })

  it('reports an unknown preset and a stale settings version', async () => {
    server.use(http.put(`${base}/api/v1/admin/settings/status-workflow`, () => problem(404, 'workflow_not_found')))
    await expect(chooseStatusWorkflow(client, { workflowId, version: 9, expectedVersion: 4 })).rejects.toMatchObject({ status: 404, code: 'workflow_not_found' })

    server.use(http.put(`${base}/api/v1/admin/settings/status-workflow`, () => problem(409, 'version_conflict')))
    await expect(chooseStatusWorkflow(client, { workflowId, version: 1, expectedVersion: 1 })).rejects.toMatchObject({ status: 409, code: 'version_conflict' })
  })
})
