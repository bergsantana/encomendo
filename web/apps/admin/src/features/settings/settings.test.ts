import type { Permission, TenantSettings } from '@encomendo/api'
import { enableAutoUnmount, flushPromises, type VueWrapper } from '@vue/test-utils'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'

import { loadPage } from '../../pageLoad'
import { button, closeApps, field, link, openApp } from '../../testing/app'
import { authSession } from '../../testing/fixtures'
import { encomendaMontagem, orderProblem, prontaEntrega, settingsUsing, workflows } from '../../testing/orders'

vi.mock('../../pageLoad', () => ({ loadPage: vi.fn() }))

const server = setupServer()
beforeAll(() => server.listen({ onUnhandledFrame: 'error' }))
afterEach(() => {
  server.resetHandlers()
  vi.mocked(loadPage).mockClear()
})
afterAll(() => server.close())
enableAutoUnmount(afterEach)
afterEach(closeApps)

const ownerPermissions: Permission[] = ['order:read', 'settings:read', 'settings:write']

const signedInAs = (role: 'owner' | 'admin' | 'staff' | 'viewer', permissions: Permission[]) =>
  server.use(http.post('*/auth/refresh', () => HttpResponse.json(authSession({ role, permissions }))))

/** serve answers the two reads the screen makes; `current` is what the store uses now. */
function serve(current: TenantSettings = settingsUsing(encomendaMontagem)) {
  server.use(
    http.get('*/readyz', () => HttpResponse.json({ status: 'ok' })),
    http.get('*/api/v1/admin/settings', () => HttpResponse.json(current)),
    http.get('*/api/v1/admin/status-workflows', () => HttpResponse.json({ items: workflows })),
  )
}

async function openSettings() {
  signedInAs('owner', ownerPermissions)
  const app = await openApp('/configuracoes')
  await vi.waitFor(() => expect(app.wrapper.text()).toContain('Lista de status'))
  return app
}

/** preset is the card of one workflow, found by its name. */
function preset(wrapper: VueWrapper, name: string) {
  const found = wrapper.findAll('article').find((card) => card.get('h3').text() === name)
  if (found === undefined) throw new Error(`no preset ${name}`)
  return found
}

const plain = (text: string) => text.replace(/\u00a0/g, ' ')

beforeEach(() => serve())

describe('the Configurações menu entry and route', () => {
  it('is in the menu for the owner', async () => {
    signedInAs('owner', ownerPermissions)

    const { wrapper } = await openApp('/hoje')

    expect(link(wrapper, 'Configurações').attributes('href')).toBe('/configuracoes')
  })

  it.each([
    ['admin', ['order:read', 'settings:read']],
    ['staff', ['order:read', 'settings:read']],
    ['viewer', ['order:read', 'settings:read']],
  ] as const)('is hidden from a %s, and its address leads to Hoje', async (role, permissions) => {
    signedInAs(role, [...permissions])

    const { wrapper, router } = await openApp('/configuracoes')

    expect(router.currentRoute.value.name).toBe('hoje')
    expect(wrapper.findAll('a').map((a) => a.text())).not.toContain('Configurações')
    expect(wrapper.text()).not.toContain('Lista de status')
  })
})

describe('Lista de status', () => {
  it('previews each preset with its staff labels, the phase badges and what the client sees', async () => {
    const { wrapper } = await openSettings()

    const montagem = preset(wrapper, 'Encomenda com montagem')
    expect(montagem.findAll('li').map((li) => plain(li.text()))).toEqual(
      encomendaMontagem.statuses.map((status) => `${status.label}O cliente vê: ${status.clientLabel}`),
    )
    expect(montagem.findAll('[data-tone]').map((badge) => badge.attributes('data-tone'))).toEqual([
      'planned',
      'planned',
      'planned',
      'waiting',
      'assembly',
      'ready',
      'delivered',
    ])
  })

  it('shows the words of whichever preset it previews, not one fixed set', async () => {
    const { wrapper } = await openSettings()

    const pronta = preset(wrapper, 'Pronta entrega')
    expect(pronta.text()).toContain('Pagar sinal')
    expect(pronta.text()).toContain('O cliente vê: Falta o sinal')
    expect(pronta.text()).not.toContain('Em montagem')
    expect(preset(wrapper, 'Encomenda com montagem').text()).not.toContain('Pagar sinal')
  })

  it('marks the one in use, offers "Usar esta lista" on the others, and says it applies to new orders', async () => {
    const { wrapper } = await openSettings()

    const current = preset(wrapper, 'Encomenda com montagem')
    expect(current.text()).toContain('Em uso')
    expect(current.findAll('button')).toHaveLength(0)
    expect(preset(wrapper, 'Pronta entrega').text()).not.toContain('Em uso')
    expect(button(preset(wrapper, 'Pronta entrega'), 'Usar esta lista').exists()).toBe(true)
    expect(wrapper.text()).toContain('Vale para os pedidos novos; os pedidos que já existem continuam com a lista em que foram criados.')
  })

  it('marks the other preset when the store uses it', async () => {
    serve(settingsUsing(prontaEntrega))

    const { wrapper } = await openSettings()

    expect(preset(wrapper, 'Pronta entrega').text()).toContain('Em uso')
    expect(preset(wrapper, 'Encomenda com montagem').text()).not.toContain('Em uso')
  })

  it('chooses a preset with its version and the settings version read, and says which one new orders use', async () => {
    let body: unknown
    server.use(
      http.put('*/api/v1/admin/settings/status-workflow', async ({ request }) => {
        body = await request.json()
        return HttpResponse.json(settingsUsing(prontaEntrega, { version: 8 }))
      }),
    )
    const { wrapper } = await openSettings()

    await button(preset(wrapper, 'Pronta entrega'), 'Usar esta lista').trigger('click')
    await flushPromises()

    expect(body).toEqual({ workflowId: prontaEntrega.id, version: prontaEntrega.version, expectedVersion: 7 })
    expect(wrapper.get('[role=status]').text()).toBe('Lista de status alterada. Os próximos pedidos usam Pronta entrega.')
    expect(preset(wrapper, 'Pronta entrega').text()).toContain('Em uso')
    expect(preset(wrapper, 'Encomenda com montagem').text()).not.toContain('Em uso')
  })

  it('sends the version the choice answered with on the next save, keeping what was typed', async () => {
    const saved: unknown[] = []
    server.use(
      http.put('*/api/v1/admin/settings/status-workflow', () => HttpResponse.json(settingsUsing(prontaEntrega, { version: 8 }))),
      http.put('*/api/v1/admin/settings', async ({ request }) => {
        saved.push(await request.json())
        return HttpResponse.json(settingsUsing(prontaEntrega, { version: 9, depositPercent: 40 }))
      }),
    )
    const { wrapper } = await openSettings()
    await field(wrapper, 'Sinal (%)').setValue('40')

    await button(preset(wrapper, 'Pronta entrega'), 'Usar esta lista').trigger('click')
    await flushPromises()
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(saved).toMatchObject([{ depositPercent: 40, version: 8 }])
  })

  it('says someone saved first and reads everything again on "Recarregar"', async () => {
    let reads = 0
    server.use(
      http.get('*/api/v1/admin/settings', () => {
        reads += 1
        return HttpResponse.json(settingsUsing(encomendaMontagem, { version: reads === 1 ? 7 : 9 }))
      }),
      http.put('*/api/v1/admin/settings/status-workflow', () => orderProblem('version_conflict')),
    )
    const { wrapper } = await openSettings()

    await button(preset(wrapper, 'Pronta entrega'), 'Usar esta lista').trigger('click')
    await flushPromises()
    expect(wrapper.get('[role=alert]').text()).toContain('Outra pessoa alterou as configurações. Recarregue para ver a versão atual.')

    await button(wrapper, 'Recarregar').trigger('click')
    await flushPromises()

    expect(reads).toBe(2)
    expect(wrapper.find('[role=alert]').exists()).toBe(false)
  })

  it('says the preset is gone when the API no longer has it', async () => {
    server.use(http.put('*/api/v1/admin/settings/status-workflow', () => HttpResponse.json({ status: 404, code: 'workflow_not_found', title: 'x' }, { status: 404 })))
    const { wrapper } = await openSettings()

    await button(preset(wrapper, 'Pronta entrega'), 'Usar esta lista').trigger('click')
    await flushPromises()

    expect(wrapper.get('[role=alert]').text()).toContain('Esta lista de status não está mais disponível.')
    expect(button(wrapper, 'Recarregar').exists()).toBe(true)
  })
})

describe('the store defaults', () => {
  it('fills the form from the settings, with the WhatsApp number as people write it', async () => {
    serve(settingsUsing(encomendaMontagem, { whatsappPhone: '5592991234567', purchaseBeforeDeposit: true, timeZone: 'America/Manaus' }))

    const { wrapper } = await openSettings()

    expect((field(wrapper, 'Sinal (%)').element as HTMLInputElement).value).toBe('50')
    expect((field(wrapper, 'Dias de Montagem').element as HTMLInputElement).value).toBe('3')
    expect((field(wrapper, 'Margem de segurança (dias)').element as HTMLInputElement).value).toBe('2')
    expect((field(wrapper, 'WhatsApp da loja').element as HTMLInputElement).value).toBe('(92) 99123-4567')
    expect((field(wrapper, 'Liberar compras antes do sinal').element as HTMLInputElement).checked).toBe(true)
    expect((field(wrapper, 'Horário do Amazonas (Manaus)').element as HTMLInputElement).checked).toBe(true)
    expect(wrapper.text()).toContain('Quanto o cliente paga para confirmar o pedido.')
    expect(wrapper.text()).toContain('Folga somada ao prazo para não prometer em cima da hora.')
    expect(wrapper.text()).toContain('Desligado: nada é comprado antes de o cliente pagar o sinal.')
    expect(wrapper.text()).toContain('Desligados, os menus e as origens ficam escondidos.')
    expect(wrapper.text()).toContain('Com DDD, como (92) 99123-4567.')
    expect(wrapper.text()).toContain('Decide quando um dia começa para os prazos e alertas.')
  })

  it('offers the four time zones with their names', async () => {
    const { wrapper } = await openSettings()

    const zones = wrapper.findAll('fieldset').find((group) => group.get('legend').text() === 'Fuso horário')
    expect(zones?.findAll('label').map((label) => label.text())).toEqual([
      'Horário de Brasília',
      'Horário do Amazonas (Manaus)',
      'Horário do Acre (Rio Branco)',
      'Horário de Fernando de Noronha',
    ])
    expect((field(wrapper, 'Horário de Brasília').element as HTMLInputElement).checked).toBe(true)
  })

  it('saves every field with the version read, the time zone and the number as digits', async () => {
    let body: unknown
    server.use(
      http.put('*/api/v1/admin/settings', async ({ request }) => {
        body = await request.json()
        return HttpResponse.json(settingsUsing(encomendaMontagem, { version: 8 }))
      }),
    )
    const { wrapper } = await openSettings()

    await field(wrapper, 'Sinal (%)').setValue('30')
    await field(wrapper, 'Dias de Montagem').setValue('4')
    await field(wrapper, 'Margem de segurança (dias)').setValue('1')
    await field(wrapper, 'WhatsApp da loja').setValue('(92) 99123-4567')
    await field(wrapper, 'Liberar compras antes do sinal').setValue(true)
    await field(wrapper, 'Usar Estoque').setValue(true)
    await field(wrapper, 'Horário do Acre (Rio Branco)').setValue(true)
    await button(wrapper, 'Salvar').trigger('submit')
    await flushPromises()

    expect(body).toEqual({
      stockEnabled: true,
      productionEnabled: false,
      purchaseBeforeDeposit: true,
      depositPercent: 30,
      assemblyDays: 4,
      safetyMarginDays: 1,
      whatsappPhone: '5592991234567',
      timeZone: 'America/Rio_Branco',
      version: 7,
    })
    expect(wrapper.get('[role=status]').text()).toBe('Configurações salvas.')
  })

  it('clears the store number when the field is emptied', async () => {
    let body: Record<string, unknown> = {}
    serve(settingsUsing(encomendaMontagem, { whatsappPhone: '5592991234567' }))
    server.use(
      http.put('*/api/v1/admin/settings', async ({ request }) => {
        body = (await request.json()) as Record<string, unknown>
        return HttpResponse.json(settingsUsing(encomendaMontagem, { version: 8 }))
      }),
    )
    const { wrapper } = await openSettings()

    await field(wrapper, 'WhatsApp da loja').setValue('')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(body.whatsappPhone).toBeNull()
  })

  it('refuses a deposit, days or number out of range before calling the API', async () => {
    let called = false
    server.use(
      http.put('*/api/v1/admin/settings', () => {
        called = true
        return HttpResponse.json(settingsUsing(encomendaMontagem))
      }),
    )
    const { wrapper } = await openSettings()

    await field(wrapper, 'Sinal (%)').setValue('101')
    await field(wrapper, 'Dias de Montagem').setValue('366')
    await field(wrapper, 'Margem de segurança (dias)').setValue('-1')
    await field(wrapper, 'WhatsApp da loja').setValue('1234')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(called).toBe(false)
    const text = wrapper.text()
    expect(text).toContain('Use um sinal de 0 a 100%.')
    expect(text.match(/Use de 0 a 365 dias\./g)).toHaveLength(2)
    expect(text).toContain('Confira o número: use DDD e 8 ou 9 dígitos.')
  })

  it('shows what the API refuses next to the field', async () => {
    server.use(
      http.put('*/api/v1/admin/settings', () =>
        HttpResponse.json(
          { status: 400, code: 'invalid', title: 'Dados inválidos', errors: [{ field: 'depositPercent', message: 'Use um sinal de 0 a 100%.' }] },
          { status: 400, headers: { 'Content-Type': 'application/problem+json' } },
        ),
      ),
    )
    const { wrapper } = await openSettings()

    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(wrapper.text()).toContain('Use um sinal de 0 a 100%.')
    expect(wrapper.get('form [role=alert]').text()).toContain('Confira os campos destacados.')
  })

  it('says someone saved first, and "Recarregar" brings their version', async () => {
    let reads = 0
    server.use(
      http.get('*/api/v1/admin/settings', () => {
        reads += 1
        return HttpResponse.json(settingsUsing(encomendaMontagem, { version: reads === 1 ? 7 : 9, depositPercent: reads === 1 ? 50 : 70 }))
      }),
      http.put('*/api/v1/admin/settings', () => orderProblem('version_conflict')),
    )
    const { wrapper } = await openSettings()
    await field(wrapper, 'Sinal (%)').setValue('30')

    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.get('form [role=alert]').text()).toContain('Outra pessoa alterou as configurações. Recarregue para ver a versão atual.')

    await button(wrapper, 'Recarregar').trigger('click')
    await flushPromises()

    expect((field(wrapper, 'Sinal (%)').element as HTMLInputElement).value).toBe('70')
    expect(wrapper.find('form [role=alert]').exists()).toBe(false)
  })
})

describe('loading the screen', () => {
  it('says it is loading, then offers "Tentar de novo" when the read fails', async () => {
    server.use(http.get('*/api/v1/admin/settings', () => new HttpResponse(null, { status: 500 })))
    signedInAs('owner', ownerPermissions)

    const { wrapper } = await openApp('/configuracoes')
    await vi.waitFor(() => expect(wrapper.text()).toContain('O servidor não respondeu.'))

    expect(button(wrapper, 'Tentar de novo').exists()).toBe(true)
    expect(wrapper.text()).not.toContain('Lista de status')
  })

  it('says "Carregando as configurações…" until the answers arrive', async () => {
    let open = () => {}
    const gate = new Promise<void>((resolve) => (open = resolve))
    server.use(
      http.get('*/api/v1/admin/settings', async () => {
        await gate
        return HttpResponse.json(settingsUsing(encomendaMontagem))
      }),
    )
    signedInAs('owner', ownerPermissions)

    const { wrapper } = await openApp('/configuracoes')
    expect(wrapper.text()).toContain('Carregando as configurações…')

    open()
    await vi.waitFor(() => expect(wrapper.text()).toContain('Lista de status'))
    expect(wrapper.text()).not.toContain('Carregando as configurações…')
  })
})

describe('the phone layout', () => {
  it('stacks the presets, and keeps the screen to one column of labelled controls', async () => {
    const { wrapper } = await openSettings()

    // Layout itself is CSS (the Playwright phone project checks the 390 px width); here, the structure it relies on.
    expect(wrapper.findAll('article')).toHaveLength(2)
    expect(wrapper.findAll('.enc-check input').length).toBeGreaterThanOrEqual(7)
    for (const control of wrapper.findAll('form input:not([type=radio]):not([type=checkbox])')) {
      expect(control.attributes('id')).toBeDefined()
      expect(wrapper.find(`label[for="${control.attributes('id')}"]`).exists()).toBe(true)
    }
    expect(button(wrapper, 'Salvar').attributes('type')).toBe('submit')
  })
})
