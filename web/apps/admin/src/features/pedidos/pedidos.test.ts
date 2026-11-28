import type { OrderSummary, Permission, StatusWorkflow } from '@encomendo/api'
import { enableAutoUnmount, flushPromises } from '@vue/test-utils'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'

import { loadPage } from '../../pageLoad'
import { button, closeApps, field, link, openApp } from '../../testing/app'
import { authSession, gate } from '../../testing/fixtures'
import { encomendaMontagem, orderSummary, prontaEntrega, settingsUsing } from '../../testing/orders'

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

const writer: Permission[] = ['order:read', 'order:write']

const signedInAs = (role: 'owner' | 'admin' | 'staff' | 'viewer', permissions: Permission[]) =>
  server.use(http.post('*/auth/refresh', () => HttpResponse.json(authSession({ role, permissions }))))

const waiting = orderSummary(encomendaMontagem, 'aguardando_sinal', { number: 'P-0231', customer: { id: 'c1', name: 'João Pereira' } })
const lateOne = orderSummary(encomendaMontagem, 'comprando', {
  id: '0190f3c4-5e1a-7b2c-8d3e-000000000202',
  number: 'P-0232',
  customer: { id: 'c2', name: 'Maria Souza' },
  alerts: [
    { kind: 'at_risk', itemId: 'i1' },
    { kind: 'late', itemId: 'i2' },
  ],
})
const riskOne = orderSummary(encomendaMontagem, 'em_montagem', {
  id: '0190f3c4-5e1a-7b2c-8d3e-000000000203',
  number: 'P-0233',
  alerts: [{ kind: 'at_risk', itemId: 'i1' }],
})
const pronta = orderSummary(prontaEntrega, 'separado', { id: '0190f3c4-5e1a-7b2c-8d3e-000000000204', number: 'P-0240', itemsReady: 2, itemsTotal: 2 })

interface Backend {
  workflow?: StatusWorkflow
  rows?: OrderSummary[]
}

/** serve answers the list and the settings; `seen` records every list query. */
function serve(backend: Backend = {}) {
  const seen: URL[] = []
  server.use(
    http.get('*/readyz', () => HttpResponse.json({ status: 'ok' })),
    http.get('*/api/v1/admin/settings', () => HttpResponse.json(settingsUsing(backend.workflow ?? encomendaMontagem))),
    http.get('*/api/v1/admin/orders', ({ request }) => {
      seen.push(new URL(request.url))
      return HttpResponse.json({ items: backend.rows ?? [waiting, lateOne, riskOne, pronta] })
    }),
  )
  return seen
}

async function openPedidos(path = '/pedidos') {
  const app = await openApp(path)
  await vi.waitFor(() => expect(app.wrapper.text()).toContain('P-0231'))
  return app
}

const plain = (text: string) => text.replace(/\u00a0/g, ' ')
const tabs = (wrapper: { findAll: (selector: string) => { text: () => string }[] }) => wrapper.findAll('[role=tab]').map((tab) => tab.text())

beforeEach(() => {
  signedInAs('owner', writer)
  serve()
})

describe('the Pedidos menu entry and route', () => {
  it('is in the menu between Hoje and Catálogo for every role that reads orders', async () => {
    signedInAs('viewer', ['order:read', 'catalog:read'])

    const { wrapper } = await openApp('/hoje')

    const entries = wrapper.findAll('nav a').map((a) => a.text())
    expect(entries.indexOf('Hoje')).toBeLessThan(entries.indexOf('Pedidos'))
    expect(entries.indexOf('Pedidos')).toBeLessThan(entries.indexOf('Catálogo'))
    expect(link(wrapper, 'Pedidos').attributes('href')).toBe('/pedidos')
  })

  it('is hidden from a role without order:read, and its address leads to Hoje', async () => {
    signedInAs('viewer', ['catalog:read'])

    const { wrapper, router } = await openApp('/pedidos')

    expect(router.currentRoute.value.name).toBe('hoje')
    expect(wrapper.findAll('a').map((a) => a.text())).not.toContain('Pedidos')
  })

  it('keeps "Pedidos" marked on the order screen and on the form', async () => {
    const { wrapper } = await openApp('/pedidos/novo')

    expect(link(wrapper, 'Pedidos').classes()).toContain('menu-link--active')
  })
})

describe('the tabs', () => {
  it("are Ativos, one per status of the store's list, Entregues and Cancelados", async () => {
    const { wrapper } = await openPedidos()

    expect(tabs(wrapper)).toEqual(['Ativos', 'Rascunho', 'Aguardando sinal', 'Confirmado', 'Comprando peças', 'Em montagem', 'Pronto', 'Entregues', 'Cancelados'])
  })

  it("follow the other workflow's own words", async () => {
    serve({ workflow: prontaEntrega, rows: [pronta] })

    const { wrapper } = await openApp('/pedidos')
    await vi.waitFor(() => expect(wrapper.text()).toContain('P-0240'))

    expect(tabs(wrapper)).toEqual(['Ativos', 'Orçamento', 'Pagar sinal', 'Separando', 'Separado', 'Entregues', 'Cancelados'])
  })

  it('read "Ativos" first, with the default scope and no status', async () => {
    const seen = serve()

    await openPedidos()

    const query = seen.at(-1)?.searchParams
    expect(query?.get('status')).toBeNull()
    expect(query?.get('scope')).toBeNull()
  })

  it('ask for the status of the tab chosen', async () => {
    const seen = serve()
    const { wrapper, router } = await openPedidos()

    await button(wrapper, 'Em montagem').trigger('click')
    await flushPromises()

    expect(seen.at(-1)?.searchParams.get('status')).toBe('em_montagem')
    expect(wrapper.get('[role=tab][aria-selected=true]').text()).toBe('Em montagem')
    expect(router.currentRoute.value.query).toEqual({ status: 'em_montagem' })
  })

  it('ask for the handed over and the cancelled scopes', async () => {
    const seen = serve()
    const { wrapper } = await openPedidos()

    await button(wrapper, 'Entregues').trigger('click')
    await flushPromises()
    expect(seen.at(-1)?.searchParams.get('scope')).toBe('handed_over')

    await button(wrapper, 'Cancelados').trigger('click')
    await flushPromises()
    expect(seen.at(-1)?.searchParams.get('scope')).toBe('cancelled')
  })

  it('move with the arrow keys, Home and End', async () => {
    const seen = serve()
    const { wrapper } = await openPedidos()
    const first = wrapper.get('[role=tab][aria-selected=true]')

    await first.trigger('keydown', { key: 'ArrowRight' })
    await flushPromises()
    expect(wrapper.get('[role=tab][aria-selected=true]').text()).toBe('Rascunho')
    expect(seen.at(-1)?.searchParams.get('status')).toBe('rascunho')

    await wrapper.get('[role=tab][aria-selected=true]').trigger('keydown', { key: 'End' })
    await flushPromises()
    expect(wrapper.get('[role=tab][aria-selected=true]').text()).toBe('Cancelados')

    await wrapper.get('[role=tab][aria-selected=true]').trigger('keydown', { key: 'Home' })
    await flushPromises()
    expect(wrapper.get('[role=tab][aria-selected=true]').text()).toBe('Ativos')
  })

  it('are still the fixed ones when the store settings cannot be read', async () => {
    server.use(http.get('*/api/v1/admin/settings', () => new HttpResponse(null, { status: 500 })))

    const { wrapper } = await openPedidos()

    expect(tabs(wrapper)).toEqual(['Ativos', 'Entregues', 'Cancelados'])
  })

  it('open from a link: a status, "Só com alerta" and a scope', async () => {
    const seen = serve()
    const { wrapper } = await openPedidos('/pedidos?status=comprando&alert=true')

    expect(wrapper.get('[role=tab][aria-selected=true]').text()).toBe('Comprando peças')
    expect((field(wrapper, 'Só com alerta').element as HTMLInputElement).checked).toBe(true)
    expect(seen.at(-1)?.searchParams.get('status')).toBe('comprando')
    expect(seen.at(-1)?.searchParams.get('alert')).toBe('true')

    const entregues = await openPedidos('/pedidos?scope=handed_over')
    expect(entregues.wrapper.get('[role=tab][aria-selected=true]').text()).toBe('Entregues')
  })

  it('open on a phase from a link, as Hoje\'s "Aguardando sinal" card does', async () => {
    const seen = serve()
    const { wrapper, router } = await openPedidos('/pedidos?phase=awaiting_deposit')

    expect(seen.at(-1)?.searchParams.get('phase')).toBe('awaiting_deposit')
    expect(wrapper.get('[role=tab][aria-selected=true]').text()).toBe('Ativos')

    // A tab chosen after that drops the phase, in the request and in the address.
    await button(wrapper, 'Confirmado').trigger('click')
    await flushPromises()
    expect(seen.at(-1)?.searchParams.get('phase')).toBeNull()
    expect(seen.at(-1)?.searchParams.get('status')).toBe('confirmado')
    expect(router.currentRoute.value.query).toEqual({ status: 'confirmado' })
  })

  it('say "Nenhum pedido encontrado." for a phase with nothing, not "Nenhum pedido ainda."', async () => {
    serve({ rows: [] })

    const { wrapper } = await openApp('/pedidos?phase=awaiting_deposit')
    await vi.waitFor(() => expect(wrapper.text()).toContain('Nenhum pedido encontrado.'))

    expect(wrapper.text()).not.toContain('Nenhum pedido ainda.')
  })

  it('show the phase as a chip in the store\'s own words, and clear it', async () => {
    const seen = serve()
    const { wrapper, router } = await openPedidos('/pedidos?phase=awaiting_deposit')

    expect(wrapper.get('.chip').text()).toBe('Aguardando sinal ×')
    await wrapper.get('.chip').trigger('click')
    await flushPromises()

    expect(wrapper.find('.chip').exists()).toBe(false)
    expect(seen.at(-1)?.searchParams.get('phase')).toBeNull()
    expect(router.currentRoute.value.query).toEqual({})
  })

  it('use the other workflow\'s words for the same phase', async () => {
    serve({ workflow: prontaEntrega, rows: [pronta] })

    const { wrapper } = await openApp('/pedidos?phase=awaiting_deposit')
    await vi.waitFor(() => expect(wrapper.find('.chip').exists()).toBe(true))

    expect(wrapper.get('.chip').text()).toBe('Pagar sinal ×')
  })

  it('name the panel by the tab that is open', async () => {
    const { wrapper } = await openPedidos()

    const open = wrapper.get('[role=tab][aria-selected=true]')
    expect(wrapper.get('[role=tabpanel]').attributes('aria-labelledby')).toBe(open.attributes('id'))

    await button(wrapper, 'Confirmado').trigger('click')
    await flushPromises()
    const next = wrapper.get('[role=tab][aria-selected=true]')
    expect(wrapper.get('[role=tabpanel]').attributes('aria-labelledby')).toBe(next.attributes('id'))
  })

  it('ignore a phase it does not know', async () => {
    const seen = serve()

    await openPedidos('/pedidos?phase=nothing')

    expect(seen.at(-1)?.searchParams.get('phase')).toBeNull()
  })

  it('open with "Só com alerta" on from ?alert=true, and write it back to the address', async () => {
    const seen = serve()
    const { wrapper, router } = await openPedidos('/pedidos?alert=true')

    expect(seen.at(-1)?.searchParams.get('alert')).toBe('true')
    expect((field(wrapper, 'Só com alerta').element as HTMLInputElement).checked).toBe(true)

    await field(wrapper, 'Só com alerta').setValue(false)
    await flushPromises()
    expect(router.currentRoute.value.query).toEqual({})

    await field(wrapper, 'Só com alerta').setValue(true)
    await flushPromises()
    expect(router.currentRoute.value.query).toEqual({ alert: 'true' })
  })

  it('keep the phase of the address while "Só com alerta" is toggled', async () => {
    const seen = serve()
    const { wrapper, router } = await openPedidos('/pedidos?phase=awaiting_deposit')

    await field(wrapper, 'Só com alerta').setValue(true)
    await flushPromises()

    expect(seen.at(-1)?.searchParams.get('phase')).toBe('awaiting_deposit')
    expect(router.currentRoute.value.query).toEqual({ phase: 'awaiting_deposit', alert: 'true' })
  })

  it('open again when a link into Pedidos is followed from Pedidos itself', async () => {
    const seen = serve()
    const { wrapper, router } = await openPedidos()

    await router.push('/pedidos?alert=true')
    await flushPromises()

    expect(seen.at(-1)?.searchParams.get('alert')).toBe('true')
    expect((field(wrapper, 'Só com alerta').element as HTMLInputElement).checked).toBe(true)
  })
})

describe('the order list', () => {
  it('lists number, customer, status, items ready, delivery and total', async () => {
    const { wrapper } = await openPedidos()

    expect(wrapper.findAll('thead th').map((th) => th.text())).toEqual(['Pedido', 'Cliente', 'Status', 'Itens prontos', 'Entrega prevista', 'Total'])
    const first = plain(wrapper.findAll('tbody tr')[0]?.text() ?? '')
    expect(first).toContain('P-0231')
    expect(first).toContain('João Pereira')
    expect(first).toContain('Aguardando sinal')
    expect(first).toContain('1 de 3')
    expect(first).toContain('12/12/2026')
    expect(first).toContain('R$ 1.299,90')
  })

  it('links each number to the order, by the route named order', async () => {
    const { wrapper, router } = await openPedidos()

    await link(wrapper, 'P-0231').trigger('click')
    await vi.waitFor(() => expect(router.currentRoute.value.name).toBe('order'))

    expect(router.currentRoute.value.name).toBe('order')
    expect(router.currentRoute.value.params.orderId).toBe(waiting.id)
  })

  it("shows labels from the order's workflow for two different workflows", async () => {
    const { wrapper } = await openPedidos()

    const rows = wrapper.findAll('tbody tr').map((row) => plain(row.text()))
    expect(rows[0]).toContain('Aguardando sinal')
    expect(rows[3]).toContain('Separado')
    // The same phase "ready" in the other list is a different word.
    expect(rows[3]).not.toContain('Pronto')
  })

  it('colors each badge by the phase, never by the label', async () => {
    const { wrapper } = await openPedidos()

    const toneOf = (row: number) => wrapper.findAll('tbody tr')[row]?.get('.enc-status-badge').attributes('data-tone')
    expect(toneOf(0)).toBe('planned')
    expect(toneOf(1)).toBe('waiting')
    expect(toneOf(2)).toBe('assembly')
    expect(toneOf(3)).toBe('ready')
  })

  it('puts the alert on top of the status: "Atrasada" over "Pode atrasar", none without one', async () => {
    const { wrapper } = await openPedidos()

    const rows = wrapper.findAll('tbody tr')
    expect(rows[0]?.find('[data-kind]').exists()).toBe(false)
    expect(rows[1]?.get('[data-kind]').text()).toBe('Atrasada')
    expect(rows[1]?.text()).toContain('Comprando peças')
    expect(rows[2]?.get('[data-kind]').text()).toBe('Pode atrasar')
  })

  it('says it is loading, then that there are no orders yet, with "Novo pedido"', async () => {
    const release = gate()
    server.use(
      http.get('*/api/v1/admin/orders', async () => {
        await release.opened
        return HttpResponse.json({ items: [] })
      }),
    )

    const { wrapper } = await openApp('/pedidos')
    expect(wrapper.text()).toContain('Carregando os pedidos…')
    release.open()
    await vi.waitFor(() => expect(wrapper.text()).toContain('Nenhum pedido ainda.'))

    expect(wrapper.findAll('button').filter((b) => b.text() === 'Novo pedido')).toHaveLength(1)
  })

  it('says "Nenhum pedido encontrado." for a search or a tab with nothing', async () => {
    const { wrapper } = await openPedidos()
    server.use(http.get('*/api/v1/admin/orders', () => HttpResponse.json({ items: [] })))

    await button(wrapper, 'Pronto').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('Nenhum pedido encontrado.')
  })

  it('reads the next page on "Carregar mais"', async () => {
    const cursors: (string | null)[] = []
    server.use(
      http.get('*/api/v1/admin/orders', ({ request }) => {
        const cursor = new URL(request.url).searchParams.get('cursor')
        cursors.push(cursor)
        return cursor === null ? HttpResponse.json({ items: [waiting], nextCursor: 'p2' }) : HttpResponse.json({ items: [pronta] })
      }),
    )
    const { wrapper } = await openPedidos()

    await button(wrapper, 'Carregar mais').trigger('click')
    await vi.waitFor(() => expect(wrapper.text()).toContain('P-0240'))

    expect(cursors).toEqual([null, 'p2'])
    expect(wrapper.text()).toContain('P-0231')
    expect(wrapper.findAll('button').map((b) => b.text())).not.toContain('Carregar mais')
  })

  it('offers "Tentar de novo" when the list cannot be read', async () => {
    server.use(http.get('*/api/v1/admin/orders', () => new HttpResponse(null, { status: 500 })))

    const { wrapper } = await openApp('/pedidos')
    await vi.waitFor(() => expect(wrapper.text()).toContain('O servidor não respondeu.'))

    expect(button(wrapper, 'Tentar de novo').exists()).toBe(true)
  })
})

describe('the search and "Só com alerta"', () => {
  it('asks for the text typed, on Enter', async () => {
    const seen = serve()
    const { wrapper } = await openPedidos()

    await field(wrapper, 'Buscar por número ou cliente').setValue('P-0231')
    await wrapper.get('form[role=search]').trigger('submit')
    await flushPromises()

    expect(seen.at(-1)?.searchParams.get('q')).toBe('P-0231')
  })

  it('asks while typing, after a short wait', async () => {
    const seen = serve()
    const { wrapper } = await openPedidos()
    const before = seen.length

    await field(wrapper, 'Buscar por número ou cliente').setValue('mar')
    expect(seen).toHaveLength(before)
    await vi.waitFor(() => expect(seen.at(-1)?.searchParams.get('q')).toBe('mar'))
  })

  it('keeps only orders with an alert when the box is ticked, and all when it is not', async () => {
    const seen = serve()
    const { wrapper } = await openPedidos()

    await field(wrapper, 'Só com alerta').setValue(true)
    await flushPromises()
    expect(seen.at(-1)?.searchParams.get('alert')).toBe('true')

    await field(wrapper, 'Só com alerta').setValue(false)
    await flushPromises()
    expect(seen.at(-1)?.searchParams.get('alert')).toBeNull()
  })

  it('keeps the tab while searching', async () => {
    const seen = serve()
    const { wrapper } = await openPedidos()
    await button(wrapper, 'Confirmado').trigger('click')
    await flushPromises()

    await field(wrapper, 'Buscar por número ou cliente').setValue('joão')
    await wrapper.get('form[role=search]').trigger('submit')
    await flushPromises()

    expect(seen.at(-1)?.searchParams.get('status')).toBe('confirmado')
    expect(seen.at(-1)?.searchParams.get('q')).toBe('joão')
  })
})

describe('"Novo pedido"', () => {
  it('opens the form for a role that writes orders', async () => {
    const { wrapper, router } = await openPedidos()

    await button(wrapper, 'Novo pedido').trigger('click')

    await vi.waitFor(() => expect(router.currentRoute.value.name).toBe('new-order'))
  })

  it('is hidden from a viewer, who still reads the list', async () => {
    signedInAs('viewer', ['order:read'])

    const { wrapper } = await openPedidos()

    expect(wrapper.findAll('button').map((b) => b.text())).not.toContain('Novo pedido')
  })
})

describe('the phone layout', () => {
  // The layout itself is CSS (the Playwright phone project checks 390 px); these are the structures it relies on.
  it('turns the list into cards: labelled cells, the number as the card title', async () => {
    const { wrapper } = await openPedidos()

    expect(wrapper.get('table').classes()).toContain('enc-cards')
    expect(wrapper.findAll('tbody tr')[0]?.findAll('td').map((cell) => cell.attributes('data-label'))).toEqual([
      'Cliente',
      'Status',
      'Itens prontos',
      'Entrega prevista',
      'Total',
    ])
    expect(wrapper.findAll('tbody tr')[0]?.find('th a').text()).toBe('P-0231')
  })

  it('keeps the tabs in one row that scrolls inside itself', async () => {
    const { wrapper } = await openPedidos()

    expect(wrapper.get('[role=tablist]').classes()).toContain('tabs')
    expect(wrapper.get('[role=tabpanel]').attributes('id')).toBe('orders-panel')
  })
})
