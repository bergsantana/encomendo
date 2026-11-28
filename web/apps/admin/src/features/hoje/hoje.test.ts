import type { PurchasingToday, Today } from '@encomendo/api'
import { enableAutoUnmount } from '@vue/test-utils'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'

import { button, closeApps, link, openApp } from '../../testing/app'
import { authSession, gate } from '../../testing/fixtures'
import { brl, encomendaMontagem, orderSummary, prontaEntrega } from '../../testing/orders'
import { emptyToday, todayHandler } from '../../testing/today'

const server = setupServer()
beforeAll(() => server.listen({ onUnhandledFrame: 'error' }))
afterEach(() => server.resetHandlers())
afterAll(() => server.close())
enableAutoUnmount(afterEach)
afterEach(closeApps)

beforeEach(() => server.use(http.post('*/auth/refresh', () => HttpResponse.json(authSession({ permissions: ['order:read'] })))))

type Need = Today['needsYou'][number]

const id = (n: number) => `0190f3c4-5e1a-7b2c-8d3e-${String(n).padStart(12, '0')}`
const itemId = id(301)

// Two orders in two stores' worth of words: the labels differ, the phases do not.
const montagem = orderSummary(encomendaMontagem, 'em_montagem', {
  id: id(201),
  number: 'P-0231',
  itemsReady: 1,
  itemsTotal: 3,
  deliveryDate: '2026-12-02',
  alerts: [{ kind: 'at_risk', itemId }],
})
const pronta = orderSummary(prontaEntrega, 'separado', {
  id: id(202),
  number: 'P-0198',
  customer: { id: id(102), name: 'Maria Souza' },
  itemsReady: 2,
  itemsTotal: 2,
  deliveryDate: '2026-12-12',
})

const needs: Need[] = [
  { kind: 'late', order: orderSummary(encomendaMontagem, 'comprando', { id: id(203), number: 'P-0240', alerts: [{ kind: 'late', itemId }] }), itemName: 'Banco de couro Scrambler', days: 2 },
  { kind: 'at_risk', order: montagem, itemName: 'Escapamento Akrapovic' },
  { kind: 'awaiting_deposit', order: orderSummary(prontaEntrega, 'pagar_sinal', { id: id(204), number: 'P-0250' }), days: 4 },
  { kind: 'ready_not_handed_over', order: pronta, days: 1 },
  { kind: 'refund_due', order: orderSummary(encomendaMontagem, 'pronto', { id: id(205), number: 'P-0260' }), amount: brl(25050) },
]

const busyDay: Today = {
  today: '2026-11-27',
  // The counts are deliberately not the length of the lists: the screen shows what the server counted.
  counts: { active: 12, awaitingDeposit: 3, withAlert: 2, dueIn7Days: 5 },
  needsYou: needs,
  upcoming: [montagem, pronta],
  truncated: false,
}

const plain = (text: string) => text.replace(/\s+/g, ' ').trim()

async function openHoje(view: Today = busyDay) {
  server.use(todayHandler(view))
  const app = await openApp('/hoje')
  await vi.waitFor(() => expect(app.wrapper.text()).not.toContain('Carregando'))
  return app
}

describe('Hoje', () => {
  it('shows the store day in the title area', async () => {
    const { wrapper } = await openHoje()

    expect(wrapper.get('h1').text()).toBe('Hoje')
    expect(wrapper.text()).toContain('sexta-feira, 27 de novembro')
  })

  it('shows the summary cards with the numbers the server counted, each opening Pedidos with its filter', async () => {
    const { wrapper } = await openHoje()

    const cards = wrapper.findAll('a.card').map((card) => [plain(card.text()), card.attributes('href')])
    expect(cards).toEqual([
      ['Pedidos ativos 12', '/pedidos'],
      ['Aguardando sinal 3', '/pedidos?phase=awaiting_deposit'],
      ['Com alerta 2', '/pedidos?alert=true'],
      ['Entregas nos próximos 7 dias 5', '/pedidos'],
    ])
  })

  it('lists the next deliveries in the server order with items ready of total and the delivery date', async () => {
    const { wrapper } = await openHoje()

    expect(wrapper.findAll('thead th').map((th) => th.text())).toEqual(['Pedido', 'Cliente', 'Status', 'Itens prontos', 'Entrega'])
    const rows = wrapper.findAll('tbody tr').map((row) => plain(row.text()))
    expect(rows[0]).toContain('P-0231')
    expect(rows[0]).toContain('João Pereira')
    expect(rows[0]).toContain('1 de 3')
    expect(rows[0]).toContain('02/12/2026')
    expect(rows[1]).toContain('P-0198')
    expect(rows[1]).toContain('Maria Souza')
    expect(rows[1]).toContain('2 de 2')
    expect(rows[1]).toContain('12/12/2026')
  })

  it("shows labels from each order's own workflow, with the colour of its phase", async () => {
    const { wrapper } = await openHoje()

    const badges = wrapper.findAll('tbody .enc-status-badge')
    expect(badges.map((badge) => badge.text())).toEqual(['Em montagem', 'Separado'])
    // assembling is the "assembly" tone and ready the "ready" tone, whatever the words.
    expect(badges.map((badge) => badge.attributes('data-tone'))).toEqual(['assembly', 'ready'])
  })

  it('shows the worst alert only, as the order list does: "Atrasada" over "Pode atrasar"', async () => {
    const mixed = orderSummary(encomendaMontagem, 'comprando', {
      id: id(203),
      number: 'P-0240',
      alerts: [
        { kind: 'at_risk', itemId },
        { kind: 'late', itemId: id(302) },
      ],
    })
    const { wrapper } = await openHoje({ ...busyDay, needsYou: [], upcoming: [mixed] })

    const row = wrapper.get('tbody tr')
    expect(row.text()).toContain('Atrasada')
    expect(row.text()).not.toContain('Pode atrasar')
  })

  it('puts the alert on top of the status: "Pode atrasar" and "Atrasada" with icon and word', async () => {
    const lateOrder = orderSummary(encomendaMontagem, 'comprando', { id: id(203), number: 'P-0240', alerts: [{ kind: 'late', itemId }, { kind: 'late', itemId: id(302) }] })
    const { wrapper } = await openHoje({ ...busyDay, needsYou: [], upcoming: [montagem, lateOrder, pronta] })

    const rows = wrapper.findAll('tbody tr')
    expect(rows[0]?.text()).toContain('Pode atrasar')
    expect(rows[0]?.text()).toContain('Em montagem')
    // Two late items of one order show one tag.
    expect(rows[1]?.text().match(/Atrasada/g)).toHaveLength(1)
    expect(rows[2]?.text()).not.toContain('atrasar')
    expect(rows[0]?.find('svg[aria-hidden="true"]').exists()).toBe(true)
  })

  it('links every next delivery to its order', async () => {
    const { wrapper } = await openHoje()

    expect(link(wrapper, 'P-0231').attributes('href')).toBe(`/pedidos/${montagem.id}`)
    expect(link(wrapper, 'P-0198').attributes('href')).toBe(`/pedidos/${pronta.id}`)
    expect(link(wrapper, 'Ver todos os pedidos').attributes('href')).toBe('/pedidos')
  })

  it('lists "Precisa de você" in the server order, each entry naming the order and customer and opening the order', async () => {
    const { wrapper } = await openHoje()

    const entries = wrapper.findAll('a.entry')
    expect(entries.map((entry) => plain(entry.text()))).toEqual([
      'P-0240 João Pereira Atrasada Banco de couro Scrambler está atrasado.',
      'P-0231 João Pereira Pode atrasar Escapamento Akrapovic pode atrasar a entrega.',
      'P-0250 João Pereira Aguardando o sinal há 4 dias.',
      'P-0198 Maria Souza Pronto há 1 dia e ainda não entregue.',
      'P-0260 João Pereira Valor a devolver: R$ 250,50.',
    ])
    expect(entries.map((entry) => entry.attributes('href'))).toEqual([id(203), id(201), id(204), id(202), id(205)].map((order) => `/pedidos/${order}`))
  })

  it('says nothing needs the owner when the list is empty', async () => {
    const { wrapper } = await openHoje({ ...busyDay, needsYou: [] })

    expect(wrapper.text()).toContain('Precisa de você')
    expect(wrapper.text()).toContain('Nada precisa de você agora.')
  })

  it('shows an empty store: zeros, no needs and no deliveries', async () => {
    const { wrapper } = await openHoje(emptyToday)

    expect(wrapper.findAll('a.card').map((card) => plain(card.text()))).toEqual([
      'Pedidos ativos 0',
      'Aguardando sinal 0',
      'Com alerta 0',
      'Entregas nos próximos 7 dias 0',
    ])
    expect(wrapper.text()).toContain('Nada precisa de você agora.')
    expect(wrapper.text()).toContain('Nenhuma entrega prevista.')
    expect(wrapper.find('table').exists()).toBe(false)
  })

  it('says it is loading until the server answers', async () => {
    const release = gate()
    server.use(
      http.get('*/api/v1/admin/today', async () => {
        await release.opened
        return HttpResponse.json(busyDay)
      }),
    )

    const { wrapper } = await openApp('/hoje')
    expect(wrapper.text()).toContain('Carregando o resumo de hoje…')
    expect(wrapper.find('a.card').exists()).toBe(false)

    release.open()
    await vi.waitFor(() => expect(wrapper.text()).toContain('Pedidos ativos'))
    expect(wrapper.text()).not.toContain('Carregando')
  })

  it('says the numbers cover only the orders it read when the server flags truncated', async () => {
    const { wrapper } = await openHoje({ ...busyDay, truncated: true })

    expect(wrapper.get('[role=status]').text()).toBe(
      'Mostrando só os 500 pedidos ativos com entrega mais próxima. Os números e os avisos desta tela cobrem só esses pedidos.',
    )
  })

  it('says nothing about truncation when the server does not flag it', async () => {
    const { wrapper } = await openHoje()

    expect(wrapper.text()).not.toContain('Mostrando só os 500')
  })

  it('tells the user when the server fails, and reads again on "Tentar de novo"', async () => {
    server.use(http.get('*/api/v1/admin/today', () => new HttpResponse(null, { status: 500 })))
    const { wrapper } = await openApp('/hoje')
    await vi.waitFor(() => expect(wrapper.find('[role=alert]').exists()).toBe(true))
    expect(wrapper.get('[role=alert]').text()).toBe('O servidor não respondeu. Tente de novo em instantes.')

    server.use(todayHandler(busyDay))
    await button(wrapper, 'Tentar de novo').trigger('click')

    await vi.waitFor(() => expect(wrapper.text()).toContain('Pedidos ativos'))
    expect(wrapper.find('[role=alert]').exists()).toBe(false)
  })

  it('shows no purchasing to a role that does not buy, and never calls it', async () => {
    let calls = 0
    server.use(
      http.get('*/api/v1/admin/purchasing/today', () => {
        calls += 1
        return HttpResponse.json({ toSend: 1, needs: [] })
      }),
    )
    const { wrapper } = await openHoje()

    expect(wrapper.text()).not.toContain('Compras')
    expect(wrapper.findAll('a.card')).toHaveLength(4)
    expect(calls).toBe(0)
  })

  it('never shows a cost, a margin or a supplier', async () => {
    const { wrapper } = await openHoje()

    for (const word of ['Custo', 'Margem', 'Fornecedor']) expect(wrapper.text()).not.toContain(word)
  })
})

describe('Hoje for a role that buys', () => {
  const needsToBuy: PurchasingToday = {
    toSend: 7,
    // The server's order: notices, then pedidos de compra with no date, then items to send.
    needs: [
      { kind: 'notify_supplier', lineId: id(601), supplierOrderId: id(501), number: 'C-0011', supplierName: 'Couros Paulista', itemName: 'Capa de banco' },
      { kind: 'no_promised_date', supplierOrderId: id(502), number: 'C-0012', supplierName: 'Moto Peças', days: 3 },
      { kind: 'to_send', supplierId: id(701), supplierName: 'Couros Paulista', count: 4 },
      { kind: 'to_send', supplierId: id(702), supplierName: 'Moto Peças', count: 1 },
    ],
  }
  let reads = 0

  beforeEach(() => {
    reads = 0
    server.use(
      http.post('*/auth/refresh', () => HttpResponse.json(authSession({ role: 'admin', permissions: ['order:read', 'purchase:write'] }))),
      http.get('*/api/v1/admin/purchasing/today', () => {
        reads += 1
        return HttpResponse.json(needsToBuy)
      }),
    )
  })

  it('adds the card "Compras a enviar", which opens Compras', async () => {
    const { wrapper } = await openHoje()

    const cards = wrapper.findAll('a.card').map((card) => [plain(card.text()), card.attributes('href')])
    expect(cards.at(-1)).toEqual(['Compras a enviar 7', '/compras'])
    expect(cards).toHaveLength(5)
    expect(reads).toBe(1)
  })

  it('puts the purchasing entries at the top of "Precisa de você", in the server order, in the product wording', async () => {
    const { wrapper } = await openHoje()

    const entries = wrapper.findAll('a.entry')
    expect(entries.slice(0, 4).map((entry) => plain(entry.text()))).toEqual([
      'Avise Couros Paulista: Capa de banco não é mais necessário (C-0011).',
      'Pedido de compra C-0012 sem prazo do fornecedor há 3 dias.',
      '4 itens para comprar de Couros Paulista.',
      '1 item para comprar de Moto Peças.',
    ])
    expect(plain(entries[4]?.text() ?? '')).toContain('P-0240')
    expect(entries).toHaveLength(4 + needs.length)
  })

  it('opens Compras from every entry: the pedido de compra without a date opens its own screen', async () => {
    const { wrapper, router } = await openHoje()

    expect(wrapper.findAll('a.entry').slice(0, 4).map((entry) => entry.attributes('href'))).toEqual(['/compras', `/compras/pedidos/${id(502)}`, '/compras', '/compras'])

    await link(wrapper, '4 itens para comprar de Couros Paulista.').trigger('click')
    await vi.waitFor(() => expect(router.currentRoute.value.name).toBe('purchasing'))
  })

  it('opens Compras from the card', async () => {
    const { wrapper, router } = await openHoje()

    await wrapper.findAll('a.card').at(-1)?.trigger('click')

    await vi.waitFor(() => expect(router.currentRoute.value.name).toBe('purchasing'))
  })

  it('does not say nothing needs the owner when only purchasing does', async () => {
    const { wrapper } = await openHoje({ ...busyDay, needsYou: [] })

    expect(wrapper.text()).not.toContain('Nada precisa de você agora.')
    expect(wrapper.findAll('a.entry')).toHaveLength(4)
  })

  it('shows a zero card and the usual empty message when purchasing has nothing', async () => {
    server.use(http.get('*/api/v1/admin/purchasing/today', () => HttpResponse.json({ toSend: 0, needs: [] })))
    const { wrapper } = await openHoje(emptyToday)

    expect(plain(wrapper.findAll('a.card').at(-1)?.text() ?? '')).toBe('Compras a enviar 0')
    expect(wrapper.text()).toContain('Nada precisa de você agora.')
  })

  it('still shows Hoje, without purchasing, when purchasing cannot be read', async () => {
    server.use(http.get('*/api/v1/admin/purchasing/today', () => new HttpResponse(null, { status: 500 })))
    const { wrapper } = await openHoje()

    expect(wrapper.findAll('a.card')).toHaveLength(4)
    expect(wrapper.find('[role=alert]').exists()).toBe(false)
    expect(wrapper.findAll('a.entry')).toHaveLength(needs.length)
  })
})

describe('Hoje on the phone', () => {
  it('turns the next deliveries into cards: every cell is labelled and the order number is the title', async () => {
    const { wrapper } = await openHoje()

    expect(wrapper.get('table').classes()).toContain('enc-cards')
    const row = wrapper.get('tbody tr')
    expect(row.findAll('td').map((cell) => cell.attributes('data-label'))).toEqual(['Cliente', 'Status', 'Itens prontos', 'Entrega'])
    expect(row.get('th[scope=row]').text()).toBe('P-0231')
  })

  it('keeps the four summary cards and every entry as links, so each is a tappable target', async () => {
    const { wrapper } = await openHoje()

    expect(wrapper.findAll('a.card')).toHaveLength(4)
    expect(wrapper.findAll('a.entry')).toHaveLength(needs.length)
  })
})
