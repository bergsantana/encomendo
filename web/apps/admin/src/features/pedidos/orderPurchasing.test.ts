import type { Order, OrderPurchase, Permission } from '@encomendo/api'
import { enableAutoUnmount, flushPromises } from '@vue/test-utils'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from 'vitest'

import { button, closeApps, link, openApp } from '../../testing/app'
import { authSession } from '../../testing/fixtures'
import { orderAsAdmin } from '../../testing/orders'
import { newOrderState, type OrderState, serveOrder } from './testkit'

vi.mock('../../pageLoad', () => ({ loadPage: vi.fn() }))

const server = setupServer()
beforeAll(() => server.listen({ onUnhandledFrame: 'error' }))
afterEach(() => {
  server.resetHandlers()
  vi.restoreAllMocks()
})
afterAll(() => server.close())
enableAutoUnmount(afterEach)
afterEach(closeApps)

const rights = {
  admin: ['order:read', 'order:write', 'order:manage', 'purchase:write', 'receiving:write'],
  staff: ['order:read', 'order:write', 'receiving:write'],
  viewer: ['order:read'],
} satisfies Record<string, Permission[]>
type Role = keyof typeof rights

const plain = (text: string) => text.replace(/\u00a0/g, ' ')
const boughtItem = orderAsAdmin().items[0] as Order['items'][number]
const withItem = (overrides: Partial<Order['items'][number]>, alerts = false): OrderState => {
  const base = orderAsAdmin()
  const item = { ...boughtItem, ...overrides, ...(alerts ? { alerts: ['at_risk' as const] } : {}) }
  return newOrderState({ ...base, items: [item] })
}

const supplierOrderId = '0190f3c4-5e1a-7b2c-8d3e-000000005001'
const chatUrl = 'https://wa.me/5511987654321?text=Ol%C3%A1%2C+Couros+Paulista%21+Sobre+o+pedido+de+compra+C-0012'

/** purchased is the item's open line: sent to Couros Paulista as C-0012, with the supplier's date. */
const purchased = (overrides: Record<string, unknown> = {}): OrderPurchase =>
  ({ orderItemId: boughtItem.id, supplierName: 'Couros Paulista', status: 'sent', lineId: 'line-1', supplierOrderId, number: 'C-0012', promisedDate: '2026-12-09', ...overrides }) as OrderPurchase
const toBuy = (overrides: Record<string, unknown> = {}): OrderPurchase => ({ orderItemId: boughtItem.id, supplierName: 'Couros Paulista', status: 'to_buy', ...overrides }) as OrderPurchase

async function openOrder(state: OrderState, role: Role = 'admin', ...extra: Parameters<typeof server.use>) {
  server.use(http.post('*/auth/refresh', () => HttpResponse.json(authSession({ role: role === 'admin' ? 'admin' : role === 'staff' ? 'staff' : 'viewer', permissions: rights[role] }))))
  serveOrder(server, state)
  server.use(...extra)
  vi.spyOn(window, 'open').mockReturnValue(null)
  const app = await openApp(`/pedidos/${state.order.id}`)
  await vi.waitFor(() => expect(app.wrapper.text()).toContain(state.order.number))
  await flushPromises()
  return app
}

const situation = (wrapper: Awaited<ReturnType<typeof openOrder>>['wrapper']) => plain(wrapper.get('.items tbody td[data-label="Situação"]').text())

describe('a bought item on the order screen', () => {
  it('shows its pedido de compra as a link for a role that buys, and the supplier\'s date', async () => {
    const state = withItem({ status: 'sent_to_supplier' })
    state.purchases = [purchased()]
    const { wrapper } = await openOrder(state)

    expect(situation(wrapper)).toContain('Enviada ao fornecedor')
    expect(situation(wrapper)).toContain('C-0012')
    expect(situation(wrapper)).toContain('Prazo do fornecedor 09/12/2026')
    expect(link(wrapper.get('.items'), 'C-0012').attributes('href')).toBe(`/compras/pedidos/${supplierOrderId}`)
  })

  it('shows the pedido de compra as plain text for a role that cannot open Compras', async () => {
    const state = withItem({ status: 'in_transit' })
    state.purchases = [purchased({ status: 'in_transit' })]
    const { wrapper } = await openOrder(state, 'staff')

    expect(situation(wrapper)).toContain('C-0012')
    expect(wrapper.get('.items').findAll('a').map((a) => a.text())).not.toContain('C-0012')
  })

  it('shows no date when the supplier gave none, and no number for an item bought at the counter', async () => {
    const state = withItem({ status: 'received', nextMoves: ['installed'] })
    state.purchases = [purchased({ status: 'received', promisedDate: null, number: null, supplierOrderId: undefined })]
    const { wrapper } = await openOrder(state)

    expect(situation(wrapper)).toBe('Recebida')
  })

  it('shows nothing from purchasing for an item the store makes', async () => {
    const state = withItem({ sourcingMode: 'in_house', status: 'planned', nextMoves: ['in_production'] })
    state.purchases = [purchased()]
    const { wrapper } = await openOrder(state)

    expect(situation(wrapper)).toBe('Planejado')
  })

  it('still works when purchasing cannot be read', async () => {
    const state = withItem({ status: 'sent_to_supplier' })
    const { wrapper } = await openOrder(state, 'admin', http.get('*/api/v1/admin/purchasing/orders/:id/lines', () => new HttpResponse(null, { status: 500 })))

    expect(situation(wrapper)).toBe('Enviada ao fornecedor')
    expect(wrapper.find('[role=alert]').exists()).toBe(false)
    expect(situation(wrapper)).not.toContain('C-0012')
  })

  describe('"Ver em Compras" and "Receber itens"', () => {
    const actions = (wrapper: Awaited<ReturnType<typeof openOrder>>['wrapper']) => wrapper.get('.items tbody td.enc-cards__actions').findAll('a').map((a) => [a.text(), a.attributes('href')])

    it('offers "Ver em Compras" while the item is "A comprar", to a role that buys', async () => {
      const state = withItem({ status: 'to_buy' })
      state.purchases = [toBuy()]
      const { wrapper } = await openOrder(state)

      expect(actions(wrapper)).toEqual([['Ver em Compras', '/compras']])
      expect(wrapper.get('.items tbody td.enc-cards__actions').findAll('button')).toHaveLength(0)
    })

    it.each(['sent_to_supplier', 'in_transit'] as const)('offers "Receber itens" once the item is %s, to a role that receives', async (status) => {
      const state = withItem({ status })
      state.purchases = [purchased({ status: status === 'in_transit' ? 'in_transit' : 'sent' })]
      const { wrapper } = await openOrder(state)

      expect(actions(wrapper)).toEqual([['Receber itens', '/compras/receber']])
    })

    it('gives staff "Receber itens" but no "Ver em Compras"', async () => {
      const waiting = withItem({ status: 'to_buy' })
      waiting.purchases = [toBuy()]
      expect(actions((await openOrder(waiting, 'staff')).wrapper)).toEqual([])

      const sent = withItem({ status: 'sent_to_supplier' })
      sent.purchases = [purchased()]
      expect(actions((await openOrder(sent, 'staff')).wrapper)).toEqual([['Receber itens', '/compras/receber']])
    })

    it('gives a viewer neither', async () => {
      const state = withItem({ status: 'sent_to_supplier' })
      state.purchases = [purchased()]
      const { wrapper } = await openOrder(state, 'viewer')

      expect(actions(wrapper)).toEqual([])
    })

    it('offers neither once the item was received', async () => {
      const state = withItem({ status: 'received', nextMoves: ['installed'] })
      state.purchases = [purchased({ status: 'received' })]
      const { wrapper } = await openOrder(state)

      expect(actions(wrapper)).toEqual([])
      expect(wrapper.get('.items tbody td.enc-cards__actions').findAll('button').map((b) => b.text())).toEqual(['Marcar como instalada'])
    })

    it('offers neither for an item the store makes', async () => {
      const state = withItem({ sourcingMode: 'in_house', status: 'planned', nextMoves: ['in_production'] })
      const { wrapper } = await openOrder(state)

      expect(actions(wrapper)).toEqual([])
      expect(button(wrapper, 'Iniciar produção').exists()).toBe(true)
    })
  })
})

describe('"Falar com o fornecedor" on the delay banner', () => {
  it('opens exactly the link the API sent, and names the supplier', async () => {
    const state = withItem({ status: 'sent_to_supplier' }, true)
    state.purchases = [purchased({ supplierChatUrl: chatUrl })]
    const { wrapper } = await openOrder(state)

    await button(wrapper.get('.banner'), 'Falar com o fornecedor (Couros Paulista)').trigger('click')

    expect(window.open).toHaveBeenCalledWith(chatUrl, '_blank', 'noopener')
  })

  it('is there also before the item was sent', async () => {
    const state = withItem({ status: 'to_buy' }, true)
    state.purchases = [toBuy({ supplierChatUrl: chatUrl })]
    const { wrapper } = await openOrder(state)

    expect(button(wrapper.get('.banner'), 'Falar com o fornecedor (Couros Paulista)').exists()).toBe(true)
  })

  it('is not there when the API sent no link, which it does only to the roles that buy', async () => {
    const state = withItem({ status: 'sent_to_supplier' }, true)
    state.purchases = [purchased()]
    const { wrapper } = await openOrder(state, 'staff')

    expect(wrapper.get('.banner').text()).toContain('Banco de couro Scrambler')
    expect(wrapper.get('.banner').text()).not.toContain('Falar com o fornecedor')
  })

  it('is not there without a delay', async () => {
    const state = withItem({ status: 'sent_to_supplier' })
    state.purchases = [purchased({ supplierChatUrl: chatUrl })]
    const { wrapper } = await openOrder(state)

    expect(wrapper.find('.banner').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('Falar com o fornecedor')
  })

  it('never opens a link that is not a wa.me link', async () => {
    const state = withItem({ status: 'sent_to_supplier' }, true)
    state.purchases = [purchased({ supplierChatUrl: 'https://example.com/phish' })]
    const { wrapper } = await openOrder(state)

    await button(wrapper.get('.banner'), 'Falar com o fornecedor (Couros Paulista)').trigger('click')

    expect(window.open).not.toHaveBeenCalled()
  })
})
