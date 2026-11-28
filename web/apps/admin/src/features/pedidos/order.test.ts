import type { HistoryEntry, Order, Permission } from '@encomendo/api'
import { enableAutoUnmount, flushPromises } from '@vue/test-utils'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'

import { loadPage } from '../../pageLoad'
import { button, closeApps, field, link, openApp } from '../../testing/app'
import { authSession, gate } from '../../testing/fixtures'
import { brl, encomendaMontagem, orderAsAdmin, orderAsOwner, orderProblem, prontaEntrega } from '../../testing/orders'
import { newOrderState, type OrderState, serveOrder } from './testkit'

vi.mock('../../pageLoad', () => ({ loadPage: vi.fn() }))

const server = setupServer()
beforeAll(() => server.listen({ onUnhandledFrame: 'error' }))
afterEach(() => {
  server.resetHandlers()
  vi.mocked(loadPage).mockClear()
  vi.restoreAllMocks()
  vi.useRealTimers()
})
afterAll(() => server.close())
enableAutoUnmount(afterEach)
afterEach(closeApps)

const ownerRights: Permission[] = ['order:read', 'order:write', 'order:manage', 'order:override', 'customer:write', 'finance:read']
const adminRights: Permission[] = ['order:read', 'order:write', 'order:manage', 'customer:write']
const staffRights: Permission[] = ['order:read', 'order:write']
const viewerRights: Permission[] = ['order:read']

type Role = 'owner' | 'admin' | 'staff' | 'viewer'
const rights: Record<Role, Permission[]> = { owner: ownerRights, admin: adminRights, staff: staffRights, viewer: viewerRights }

const signedInAs = (role: Role) => server.use(http.post('*/auth/refresh', () => HttpResponse.json(authSession({ role, permissions: rights[role] }))))

const plain = (text: string) => text.replace(/\u00a0/g, ' ')
const orderId = orderAsAdmin().id
const whatsapp = 'https://wa.me/5592991234567?text=data'

const boughtItem = orderAsAdmin().items[0] as Order['items'][number]
const withItems = (order: Order, items: Order['items']): Order => ({ ...order, items })
const item = (overrides: Partial<Order['items'][number]>): Order['items'][number] => ({ ...boughtItem, ...overrides })

async function openOrder(state: OrderState, role: Role = 'owner') {
  signedInAs(role)
  serveOrder(server, state)
  vi.spyOn(window, 'open').mockReturnValue(null)
  const app = await openApp(`/pedidos/${state.order.id}`)
  await vi.waitFor(() => expect(app.wrapper.text()).toContain(state.order.number))
  return app
}

const asAdmin = (statusKey = 'aguardando_sinal', overrides: Partial<Order> = {}) => newOrderState(orderAsAdmin(encomendaMontagem, statusKey, overrides))
const asOwner = (statusKey = 'aguardando_sinal', overrides: Partial<Order> = {}) => newOrderState(orderAsOwner(encomendaMontagem, statusKey, overrides))

beforeEach(() => {
  signedInAs('owner')
})

describe('the route', () => {
  it('opens by its name and parameter, and a role that reads orders sees it', async () => {
    const { router } = await openOrder(asAdmin(), 'viewer')

    expect(router.currentRoute.value.name).toBe('order')
    expect(router.currentRoute.value.params.orderId).toBe(orderId)
  })

  it('says "Pedido não encontrado." for an order that is not there, with a way back', async () => {
    signedInAs('owner')
    serveOrder(server, asAdmin())
    server.use(http.get('*/api/v1/admin/orders/:id', () => orderProblem('not_found')))

    const { wrapper } = await openApp('/pedidos/inexistente')
    await vi.waitFor(() => expect(wrapper.text()).toContain('Pedido não encontrado.'))

    expect(link(wrapper, 'Voltar para Pedidos').attributes('href')).toBe('/pedidos')
    expect(button(wrapper, 'Tentar de novo').exists()).toBe(true)
  })

  it('says it is loading', async () => {
    signedInAs('owner')
    serveOrder(server, asAdmin())
    const release = gate()
    server.use(
      http.get('*/api/v1/admin/orders/:id', async () => {
        await release.opened
        return HttpResponse.json(orderAsAdmin())
      }),
    )

    const { wrapper } = await openApp(`/pedidos/${orderId}`)

    expect(wrapper.text()).toContain('Carregando o pedido…')
    release.open()
    await vi.waitFor(() => expect(wrapper.text()).toContain('P-0231'))
  })
})

describe('the header', () => {
  it('shows the number, the status, the customer and when it was created', async () => {
    const { wrapper } = await openOrder(asAdmin())

    expect(plain(wrapper.get('h1').text())).toBe('Pedido P-0231')
    expect(wrapper.get('.order .header .enc-status-badge').text()).toBe('Aguardando sinal')
    expect(plain(wrapper.get('.order .header').text())).toContain('João Pereira')
    expect(plain(wrapper.get('.order .header').text())).toContain('(92) 99123-4567')
    expect(wrapper.get('.order .header').text()).toContain('Criado em 27/11/2026')
  })

  it("shows the day the store lived for 'Criado em'", async () => {
    // 02:30 UTC on the 28th is still the 27th in Manaus (UTC-4).
    const state = asAdmin('aguardando_sinal', { createdAt: '2026-11-28T02:30:00Z' })
    state.timeZone = 'America/Manaus'

    const { wrapper } = await openOrder(state)

    expect(wrapper.get('.order .header').text()).toContain('Criado em 27/11/2026')
  })

  it('shows labels from the order\'s workflow for two different workflows', async () => {
    const montagem = await openOrder(newOrderState(orderAsAdmin(encomendaMontagem, 'comprando')))
    expect(montagem.wrapper.get('.order .header .enc-status-badge').text()).toBe('Comprando peças')
    expect(montagem.wrapper.get('.order .header .enc-status-badge').attributes('data-tone')).toBe('waiting')

    const pronta = await openOrder(newOrderState(orderAsAdmin(prontaEntrega, 'separado')))
    expect(pronta.wrapper.get('.order .header .enc-status-badge').text()).toBe('Separado')
    expect(pronta.wrapper.get('.order .header .enc-status-badge').attributes('data-tone')).toBe('ready')
  })

  it('offers "Próximo status" only with the labels of its own list', async () => {
    const { wrapper } = await openOrder(newOrderState(orderAsAdmin(prontaEntrega, 'orcamento', { moves: [prontaEntrega.statuses[1] as (typeof prontaEntrega.statuses)[number]] })))

    expect(wrapper.findAll('.advance button').map((b) => b.text())).toEqual(['Mover para Pagar sinal'])
  })
})

describe('"Entrega"', () => {
  it('shows the promised date, the date it must arrive by and a pickup', async () => {
    const { wrapper } = await openOrder(asAdmin())

    const text = plain(wrapper.get('.delivery').text())
    expect(text).toContain('Entrega prevista: 12/12/2026')
    expect(text).toContain('Precisa chegar até: 09/12/2026')
    expect(text).toContain('Retirada na loja')
  })

  it('names the address of a delivery', async () => {
    const state = asAdmin('aguardando_sinal', {
      handover: 'delivery',
      address: { id: 'a1', label: 'Casa', cep: '69057000', street: 'Rua das Palmeiras', number: '120', complement: '', district: 'Adrianópolis', city: 'Manaus', state: 'AM' },
    })

    const { wrapper } = await openOrder(state)

    expect(wrapper.get('.delivery').text()).toContain('Entrega em Rua das Palmeiras, 120 - Adrianópolis - Manaus/AM')
  })

  it('offers "Alterar data" to the owner and an admin, not to staff or a viewer', async () => {
    for (const [role, shown] of [['owner', true], ['admin', true], ['staff', false], ['viewer', false]] as const) {
      const { wrapper } = await openOrder(asAdmin(), role)
      expect(wrapper.get('.delivery').findAll('button').map((b) => b.text()).includes('Alterar data'), role).toBe(shown)
    }
  })
})

describe('the delay banner', () => {
  const late = item({ id: 'late-1', name: 'Banco de couro Scrambler', alerts: ['late'], expectedDate: '2026-12-10', backupSupplier: { name: 'Couros Sul', leadDays: 6 } })
  const risk = item({ id: 'risk-1', name: 'Guidão baixo', alerts: ['at_risk'], expectedDate: '2026-12-11' })

  it('says how many items may delay the delivery, with each expected date against "Precisa chegar até"', async () => {
    const { wrapper } = await openOrder(newOrderState(withItems(orderAsAdmin(), [risk])))

    const banner = wrapper.get('.banner')
    expect(banner.get('h2').text()).toBe('1 item pode atrasar a entrega')
    expect(banner.text()).toContain('Guidão baixo: previsto para 11/12/2026, precisa chegar até 09/12/2026.')
  })

  it('uses the plural, and says "atrasado" for a late item', async () => {
    const two = item({ id: 'risk-2', name: 'Farol LED', alerts: ['at_risk'] })
    const many = await openOrder(newOrderState(withItems(orderAsAdmin(), [risk, two])))
    expect(many.wrapper.get('.banner h2').text()).toBe('2 itens podem atrasar a entrega')

    const one = await openOrder(newOrderState(withItems(orderAsAdmin(), [late])))
    expect(one.wrapper.get('.banner h2').text()).toBe('1 item atrasado')

    const late2 = item({ id: 'late-2', name: 'Farol LED', alerts: ['late'] })
    const both = await openOrder(newOrderState(withItems(orderAsAdmin(), [late, late2, risk])))
    expect(both.wrapper.get('.banner h2').text()).toContain('2 itens atrasados')
    expect(both.wrapper.get('.banner h2').text()).toContain('1 item pode atrasar a entrega')
  })

  it('is not there when no item has an alert', async () => {
    const { wrapper } = await openOrder(asAdmin())

    expect(wrapper.find('.banner').exists()).toBe(false)
  })

  it('offers the backup supplier, with its name and days, to the owner and an admin', async () => {
    const { wrapper } = await openOrder(newOrderState(withItems(orderAsAdmin(), [late])), 'admin')

    expect(button(wrapper, 'Usar o fornecedor reserva (Couros Sul, 6 dias)').exists()).toBe(true)
    expect(button(wrapper, 'Alterar a data de entrega').exists()).toBe(true)
  })

  it('hides the fixes from staff, and the backup button once the item was bought or has no backup', async () => {
    const staff = await openOrder(newOrderState(withItems(orderAsAdmin(), [late])), 'staff')
    expect(staff.wrapper.get('.banner').findAll('button')).toHaveLength(0)

    const bought = await openOrder(newOrderState(withItems(orderAsAdmin(), [{ ...late, status: 'sent_to_supplier' }])), 'admin')
    expect(bought.wrapper.get('.banner').text()).not.toContain('Usar o fornecedor reserva')

    const without = await openOrder(newOrderState(withItems(orderAsAdmin(), [risk])), 'admin')
    expect(without.wrapper.get('.banner').text()).not.toContain('Usar o fornecedor reserva')
  })

  it('switches to the backup supplier and says where the item comes from now', async () => {
    const state = newOrderState(withItems(orderAsAdmin(), [late]))
    state.answers.backup = () => {
      const switched = { ...late, alerts: [], expectedDate: '2026-12-06', supplier: { name: 'Couros Sul', leadDays: 6 } }
      state.order = withItems(state.order, [switched])
      return HttpResponse.json(state.order)
    }
    const { wrapper } = await openOrder(state, 'admin')

    await button(wrapper, 'Usar o fornecedor reserva (Couros Sul, 6 dias)').trigger('click')
    await vi.waitFor(() => expect(wrapper.text()).toContain('Banco de couro Scrambler agora vem de Couros Sul. Previsto: 06/12/2026.'))

    expect(state.calls.backup?.[0]?.body).toEqual({ version: 1 })
    expect(wrapper.find('.banner').exists()).toBe(false)
  })

  it('shows the refusal when the item has no backup any more', async () => {
    const state = newOrderState(withItems(orderAsAdmin(), [late]))
    state.answers.backup = () => orderProblem('no_backup_supplier')
    const { wrapper } = await openOrder(state, 'admin')

    await button(wrapper, 'Usar o fornecedor reserva (Couros Sul, 6 dias)').trigger('click')
    await flushPromises()

    expect(wrapper.get('.alert').text()).toBe('Este item não tem fornecedor reserva ou já foi comprado.')
  })

  it('opens "Alterar data" from "Alterar a data de entrega"', async () => {
    const { wrapper } = await openOrder(newOrderState(withItems(orderAsAdmin(), [late])), 'admin')

    await button(wrapper, 'Alterar a data de entrega').trigger('click')

    expect(wrapper.get('[role=dialog]').attributes('aria-labelledby')).toBeDefined()
    expect(wrapper.get('[role=dialog] h2').text()).toBe('Alterar data')
  })
})

describe('the timeline', () => {
  it('marks today, the arrival limit, the assembly and the delivery with their dates', async () => {
    vi.useFakeTimers({ toFake: ['Date'], now: new Date('2026-11-28T02:00:00Z') })
    const state = asAdmin()
    state.timeZone = 'America/Manaus'

    const { wrapper } = await openOrder(state)

    const legend = wrapper.findAll('.timeline .legend li').map((li) => plain(li.text()))
    expect(legend).toEqual(['Hoje: 27/11/2026', 'Precisa chegar até: 09/12/2026', 'Montagem: 09/12/2026 a 12/12/2026', 'Entrega: 12/12/2026'])
  })

  it('shows "Previsto" for an item on its way and "Recebido em" for one that arrived', async () => {
    const arrived = item({ id: 'arrived', name: 'Farol LED', status: 'received', arrivedOn: '2026-12-01', arrived: true, check: 'ok', nextMoves: ['installed'] })
    const { wrapper } = await openOrder(newOrderState(withItems(orderAsAdmin(), [boughtItem, arrived])))

    const rows = wrapper.findAll('.timeline .row').map((row) => plain(row.text()))
    expect(rows[0]).toContain('Banco de couro Scrambler')
    expect(rows[0]).toContain('Previsto: 05/12/2026')
    expect(rows[1]).toContain('Recebido em 01/12/2026')
  })

  it('draws an item with an alert in the alert color, as the API says', async () => {
    const { wrapper } = await openOrder(newOrderState(withItems(orderAsAdmin(), [boughtItem, item({ id: 'late', alerts: ['late'], name: 'Farol LED' })])))

    expect(wrapper.findAll('.timeline .row').map((row) => row.attributes('data-alert'))).toEqual(['false', 'true'])
  })

  it('keeps the band for the desktop and the dates as text for the phone', async () => {
    const { wrapper } = await openOrder(asAdmin())

    // The phone layout is CSS: the band is hidden and the same dates stay in the rows.
    expect(wrapper.find('.timeline .band').exists()).toBe(true)
    expect(wrapper.get('.timeline .row .date').text()).toContain('05/12/2026')
  })
})

describe('"Abrir conversa no WhatsApp"', () => {
  it('opens the chat link the API sent with the order, not one made here', async () => {
    const state = asAdmin('aguardando_sinal', { customer: { ...orderAsAdmin().customer, whatsappUrl: 'https://wa.me/5592991234567' } })
    const { wrapper } = await openOrder(state, 'viewer')

    await button(wrapper.get('.order .header'), 'Abrir conversa no WhatsApp').trigger('click')

    expect(window.open).toHaveBeenCalledOnce()
    expect(window.open).toHaveBeenCalledWith('https://wa.me/5592991234567', '_blank', 'noopener')
  })
})

describe('"Itens"', () => {
  it('lists item, origin, situation and expected date', async () => {
    const { wrapper } = await openOrder(asAdmin())

    const table = wrapper.get('.items table')
    expect(table.findAll('thead th').map((th) => th.text())).toEqual(['Item', 'Origem', 'Situação', 'Previsto', 'Ações'])
    const row = plain(table.get('tbody tr').text())
    expect(row).toContain('Banco de couro Scrambler')
    expect(row).toContain('Sob encomenda')
    expect(row).toContain('A comprar')
    expect(row).toContain('05/12/2026')
  })

  it('names the situation by the sourcing mode: Recebida and Instalada, Pronto and Instalado', async () => {
    const received = item({ id: 'a', name: 'Banco', status: 'received', nextMoves: ['installed'] })
    const inHouse = item({ id: 'b', name: 'Painel', sourcingMode: 'in_house', status: 'ready', nextMoves: ['installed'] })
    const installed = item({ id: 'c', name: 'Painel 2', sourcingMode: 'in_house', status: 'installed', nextMoves: [] })
    const { wrapper } = await openOrder(newOrderState(withItems(orderAsAdmin(), [received, inHouse, installed])))

    const badges = wrapper.findAll('.items tbody .enc-status-badge').map((badge) => badge.text())
    expect(badges).toEqual(['Recebida', 'Pronto', 'Instalado'])
    expect(wrapper.findAll('.items tbody tr').map((row) => plain(row.text()))[1]).toContain('Produção própria')
  })

  it('shows the supplier and the chosen options', async () => {
    const chosen = item({
      supplier: { name: 'Couros Paulista', leadDays: 15 },
      options: [{ optionId: 'o', choiceId: 'c', name: 'Cor', label: 'Preto', extraPrice: brl(0), extraDays: 0 }],
    })
    const { wrapper } = await openOrder(newOrderState(withItems(orderAsAdmin(), [chosen])))

    const row = plain(wrapper.get('.items tbody tr').text())
    expect(row).toContain('Couros Paulista')
    expect(row).toContain('Cor: Preto')
  })

  it.each([
    ['installed', 'Marcar como instalada'],
    ['in_production', 'Iniciar produção'],
  ] as const)('moves an item to %s with "%s"', async (to, label) => {
    const state = newOrderState(withItems(orderAsAdmin(), [item({ nextMoves: [to] })]))
    const { wrapper } = await openOrder(state)

    await button(wrapper, label).trigger('click')
    await flushPromises()

    expect(state.calls.move?.[0]?.body).toEqual({ to, version: 1 })
  })

  it('says "instalado" for an in-house item', async () => {
    const { wrapper } = await openOrder(newOrderState(withItems(orderAsAdmin(), [item({ sourcingMode: 'in_house', status: 'ready', nextMoves: ['installed'] })])))

    expect(button(wrapper, 'Marcar como instalado').exists()).toBe(true)
  })

  it('offers a received bought item only "Marcar como instalada": sending and receiving belong to Compras', async () => {
    const { wrapper } = await openOrder(newOrderState(withItems(orderAsAdmin(), [item({ status: 'received', nextMoves: ['installed'] })])))

    expect(wrapper.get('.items tbody td.enc-cards__actions').findAll('button').map((b) => b.text())).toEqual(['Marcar como instalada'])
  })

  it.each([
    ['to_buy', 'A comprar'],
    ['sent_to_supplier', 'Enviada ao fornecedor'],
    ['in_transit', 'Em trânsito'],
  ] as const)('draws no move button for a bought item that is %s, even if a move came with it', async (status, word) => {
    const stale = item({ status, nextMoves: ['sent_to_supplier', 'in_transit', 'received'] })
    const { wrapper } = await openOrder(newOrderState(withItems(orderAsAdmin(), [stale])))

    expect(wrapper.get('.items tbody .enc-status-badge').text()).toBe(word)
    expect(wrapper.get('.items tbody td.enc-cards__actions').findAll('button')).toHaveLength(0)
    expect(wrapper.text()).not.toContain('Marcar como enviada ao fornecedor')
    expect(wrapper.text()).not.toContain('Marcar como em trânsito')
  })

  it('says purchasing moves the item when the API refuses a manual move for that reason', async () => {
    const state = newOrderState(withItems(orderAsAdmin(), [item({ status: 'received', nextMoves: ['installed'] })]))
    state.answers.move = () => orderProblem('item_moved_by_purchasing')
    const { wrapper } = await openOrder(state)

    await button(wrapper, 'Marcar como instalada').trigger('click')
    await flushPromises()

    expect(wrapper.get('.alert').text()).toBe('Este item é movido por Compras. Use Compras ou Receber itens.')
  })

  it('says "Em produção" for a defective in-house item', async () => {
    const made = item({ sourcingMode: 'in_house', name: 'Painel', status: 'in_production', nextMoves: ['ready'] })
    const state = newOrderState(withItems(orderAsAdmin(), [made]))
    state.answers.move = () => {
      state.order = withItems(state.order, [{ ...made, check: 'defective' }])
      return HttpResponse.json(state.order)
    }
    const { wrapper } = await openOrder(state)

    await button(wrapper, 'Marcar como pronto').trigger('click')
    await button(wrapper, 'Com defeito').trigger('click')
    await vi.waitFor(() => expect(wrapper.text()).toContain('Painel voltou para Em produção.'))

    expect(state.calls.move?.[0]?.body).toEqual({ to: 'ready', check: 'defective', version: 1 })
  })

  it('asks "Tudo certo" or "Com defeito" after "Marcar como pronto"', async () => {
    const state = newOrderState(withItems(orderAsAdmin(), [item({ sourcingMode: 'in_house', status: 'in_production', nextMoves: ['ready'] })]))
    const { wrapper } = await openOrder(state)

    await button(wrapper, 'Marcar como pronto').trigger('click')
    expect(wrapper.get('.items tbody td.enc-cards__actions').findAll('button').map((b) => b.text())).toEqual(['Tudo certo', 'Com defeito', 'Voltar'])
    await button(wrapper, 'Tudo certo').trigger('click')
    await flushPromises()

    expect(state.calls.move?.[0]?.body).toEqual({ to: 'ready', check: 'ok', version: 1 })
  })

  it('goes back from "Tudo certo" and "Com defeito" to "Marcar como pronto", moving nothing', async () => {
    const state = newOrderState(withItems(orderAsAdmin(), [item({ sourcingMode: 'in_house', status: 'in_production', nextMoves: ['ready'] })]))
    const { wrapper } = await openOrder(state)
    await button(wrapper, 'Marcar como pronto').trigger('click')

    await button(wrapper, 'Voltar').trigger('click')

    expect(wrapper.get('.items tbody td.enc-cards__actions').findAll('button').map((b) => b.text())).toEqual(['Marcar como pronto'])
    expect(state.calls.move).toBeUndefined()
  })

  it('shows the refusal of a move', async () => {
    const state = newOrderState(withItems(orderAsAdmin(), [item({ status: 'received', nextMoves: ['installed'] })]))
    state.answers.move = () => orderProblem('invalid_item_move')
    const { wrapper } = await openOrder(state)

    await button(wrapper, 'Marcar como instalada').trigger('click')
    await flushPromises()

    expect(wrapper.get('.alert').text()).toBe('O item não pode ir para este estado.')
  })

  it('gives a viewer no actions', async () => {
    const { wrapper } = await openOrder(newOrderState(withItems(orderAsAdmin(), [item({ status: 'received', nextMoves: ['installed'] })])), 'viewer')

    expect(wrapper.get('.items tbody td.enc-cards__actions').findAll('button')).toHaveLength(0)
  })

  it('shows cost per item only when the API sent it (owner), never for an admin', async () => {
    const owner = await openOrder(asOwner())
    expect(owner.wrapper.findAll('.items thead th').map((th) => th.text())).toContain('Custo')
    expect(plain(owner.wrapper.get('.items tbody tr').text())).toContain('R$ 900,00')

    const admin = await openOrder(asAdmin(), 'admin')
    expect(admin.wrapper.findAll('.items thead th').map((th) => th.text())).not.toContain('Custo')
  })
})

describe('"Próximo status"', () => {
  it('offers one button per status the API allows', async () => {
    const state = asAdmin()
    const moves = state.order.moves.slice(0, 2)
    state.order = { ...state.order, moves }
    const { wrapper } = await openOrder(state)

    expect(wrapper.findAll('.advance .buttons button').map((b) => b.text())).toEqual(moves.map((move) => `Mover para ${move.label}`))
  })

  it('moves the order and shows the status it answered', async () => {
    const state = asAdmin()
    state.answers.advance = () => {
      state.order = orderAsAdmin(encomendaMontagem, 'confirmado', { version: 2 })
      return HttpResponse.json(state.order)
    }
    const { wrapper } = await openOrder(state)

    await button(wrapper, 'Mover para Confirmado').trigger('click')
    await vi.waitFor(() => expect(wrapper.get('.order .header .enc-status-badge').text()).toBe('Confirmado'))

    expect(state.calls.advance?.[0]?.body).toEqual({ to: 'confirmado', version: 1 })
  })

  it.each([
    ['deposit_unpaid', 'O sinal ainda não foi pago. Registre o sinal ou libere o pedido sem ele.'],
    ['items_not_ready', 'Nem todos os itens chegaram e foram conferidos.'],
    ['balance_unpaid', 'Falta receber o saldo de R$ 1.299,90.'],
  ] as const)('shows each guard message: %s', async (guard, message) => {
    const state = asAdmin()
    state.answers.advance = () => orderProblem('guard_failed', { guard })
    const { wrapper } = await openOrder(state)

    await button(wrapper, 'Mover para Confirmado').trigger('click')
    await flushPromises()

    expect(plain(wrapper.get('.advance [role=alert]').text())).toBe(message)
  })

  it('says the order cannot go to that status when the transition is refused', async () => {
    const state = asAdmin()
    state.answers.advance = () => orderProblem('invalid_transition')
    const { wrapper } = await openOrder(state)

    await button(wrapper, 'Mover para Em montagem').trigger('click')
    await flushPromises()

    expect(wrapper.get('.advance [role=alert]').text()).toBe('Este pedido não pode ir para Em montagem.')
  })

  it('clears the guard message on the next try', async () => {
    const state = asAdmin()
    state.answers.advance = () => orderProblem('guard_failed', { guard: 'deposit_unpaid' })
    const { wrapper } = await openOrder(state)
    await button(wrapper, 'Mover para Confirmado').trigger('click')
    await flushPromises()
    expect(wrapper.find('.advance [role=alert]').exists()).toBe(true)

    delete state.answers.advance
    await button(wrapper, 'Mover para Confirmado').trigger('click')
    await flushPromises()

    expect(wrapper.find('.advance [role=alert]').exists()).toBe(false)
  })

  it('asks "Como o pedido foi entregue?" before the status that hands the order over', async () => {
    const state = asAdmin('pronto')
    const { wrapper } = await openOrder(state)

    await button(wrapper, 'Mover para Entregue').trigger('click')
    expect(wrapper.get('[role=dialog] h2').text()).toBe('Como o pedido foi entregue?')
    const dialog = wrapper.get('[role=dialog]')
    expect((field(dialog, 'Retirada na loja').element as HTMLInputElement).checked).toBe(true)
    await field(dialog, 'Entrega').setValue(true)
    await button(dialog, 'Mover para Entregue').trigger('click')
    await flushPromises()

    expect(state.calls.advance?.[0]?.body).toEqual({ to: 'entregue', handover: 'delivery', version: 1 })
    expect(wrapper.find('[role=dialog]').exists()).toBe(false)
  })

  it('keeps the handover question open and the guard message above the buttons when the balance is due', async () => {
    const state = asAdmin('pronto')
    state.answers.advance = () => orderProblem('guard_failed', { guard: 'balance_unpaid' })
    const { wrapper } = await openOrder(state)

    await button(wrapper, 'Mover para Entregue').trigger('click')
    await button(wrapper.get('[role=dialog]'), 'Mover para Entregue').trigger('click')
    await flushPromises()

    expect(plain(wrapper.get('.advance [role=alert]').text())).toContain('Falta receber o saldo de')
    // The sheet covers the page on a phone, so the dialog says it too.
    expect(plain(wrapper.get('[role=dialog] [role=alert]').text())).toContain('Falta receber o saldo de R$ 1.299,90.')
  })

  it('is hidden from a viewer', async () => {
    const { wrapper } = await openOrder(asAdmin(), 'viewer')

    expect(wrapper.find('.advance').exists()).toBe(false)
  })

  it('is gone once the order was delivered or cancelled', async () => {
    const { wrapper } = await openOrder(newOrderState(orderAsAdmin(encomendaMontagem, 'entregue', { moves: [] })))

    expect(wrapper.find('.advance').exists()).toBe(false)
    expect(wrapper.findAll('button').map((b) => b.text())).not.toContain('Cancelar pedido')
  })
})

describe('"Pagamentos"', () => {
  const paid = (extra: Partial<Order> = {}): Partial<Order> => ({
    paid: brl(64995),
    balanceDue: brl(64995),
    depositPaid: true,
    payments: [
      {
        id: 'p1',
        kind: 'deposit',
        amount: brl(64995),
        method: 'pix',
        recordedBy: { id: 'u1', name: 'Ana Souza' },
        recordedAt: '2026-11-27T15:00:00Z',
      },
    ],
    ...extra,
  })

  it('shows total, deposit, paid and balance as the API computed them', async () => {
    const { wrapper } = await openOrder(asAdmin('aguardando_sinal', paid()))

    const panel = plain(wrapper.get('.payments dl').text())
    expect(panel).toContain('TotalR$ 1.299,90')
    expect(panel).toContain('SinalR$ 649,95 (50%)')
    expect(panel).toContain('PagoR$ 649,95')
    expect(panel).toContain('SaldoR$ 649,95')
    expect(panel).not.toContain('Valor a devolver')
  })

  it('lists each payment with date, type, method, amount and who recorded it', async () => {
    const { wrapper } = await openOrder(asAdmin('aguardando_sinal', paid()))

    const row = plain(wrapper.get('.payments tbody tr').text())
    expect(wrapper.findAll('.payments thead th').map((th) => th.text())).toEqual(['Data', 'Tipo', 'Forma', 'Valor', 'Registrado por', 'Ações'])
    expect(row).toContain('27/11/2026')
    expect(row).toContain('Sinal')
    expect(row).toContain('Pix')
    expect(row).toContain('R$ 649,95')
    expect(row).toContain('Ana Souza')
  })

  it('shows "Valor a devolver" when the API says there is one', async () => {
    const { wrapper } = await openOrder(asAdmin('aguardando_sinal', paid({ refundDue: brl(10000) })))

    expect(plain(wrapper.get('.payments dl').text())).toContain('Valor a devolverR$ 100,00')
  })

  it('marks a voided payment "Anulado" with its reason', async () => {
    const first = (paid().payments ?? [])[0]
    if (first === undefined) throw new Error('fixture')
    const voided = { ...first, voided: { by: { id: 'u', name: 'Ana' }, at: '2026-11-27T16:00:00Z', reason: 'Valor errado' } }
    const { wrapper } = await openOrder(asAdmin('aguardando_sinal', { payments: [voided] }))

    expect(wrapper.get('.payments tbody tr').text()).toContain('Anulado: Valor errado')
  })

  it('shows cost and margin to the owner, and neither to an admin', async () => {
    const owner = await openOrder(asOwner())
    const text = plain(owner.wrapper.get('.payments dl').text())
    expect(text).toContain('CustoR$ 900,00')
    expect(text).toContain('MargemR$ 399,90 (30,76%)')

    const admin = await openOrder(asAdmin(), 'admin')
    expect(admin.wrapper.get('.payments dl').text()).not.toContain('Custo')
    expect(admin.wrapper.get('.payments dl').text()).not.toContain('Margem')
  })

  it('shows "—" for the owner when the order has no cost to give', async () => {
    const state = asOwner()
    state.order = { ...state.order, cost: null, margin: null, items: state.order.items.map((entry) => ({ ...entry, cost: null })) }
    const { wrapper } = await openOrder(state, 'owner')

    expect(plain(wrapper.get('.payments dl').text())).toContain('Custo—')
    expect(plain(wrapper.get('.payments dl').text())).toContain('Margem—')
    expect(wrapper.findAll('.items thead th').map((th) => th.text())).toContain('Custo')
    expect(wrapper.get('.items tbody tr').text()).toContain('—')
  })

  it('hides cost and margin from an admin even when only the owner copy is open before', async () => {
    const { wrapper } = await openOrder(asAdmin(), 'admin')

    expect(wrapper.text()).not.toContain('Custo')
    expect(wrapper.text()).not.toContain('Margem')
  })

  it('opens "Registrar pagamento" with the deposit due, and records it with the version', async () => {
    const state = asAdmin()
    const { wrapper } = await openOrder(state, 'staff')

    await button(wrapper, 'Registrar pagamento').trigger('click')
    expect((field(wrapper, 'Tipo').element as HTMLSelectElement).value).toBe('deposit')
    expect((field(wrapper, 'Valor').element as HTMLInputElement).value).toBe('649,95')
    expect(field(wrapper, 'Forma').findAll('option').map((o) => o.text())).toEqual(['Pix', 'Dinheiro', 'Cartão de crédito', 'Cartão de débito', 'Transferência', 'Outro'])
    await field(wrapper, 'Forma').setValue('dinheiro')
    await button(wrapper, 'Registrar').trigger('click')
    await flushPromises()

    expect(state.calls.payment?.[0]?.body).toEqual({ kind: 'deposit', amountCents: 64995, method: 'dinheiro', version: 1 })
    expect(state.calls.payment?.[0]?.key).toMatch(/^[0-9a-f-]{36}$/)
    expect(wrapper.find('[role=dialog]').exists()).toBe(false)
    expect(wrapper.text()).toContain('Pagamento registrado.')
  })

  it('fills Sinal with what is still owed of the deposit, not the whole deposit', async () => {
    // Half of the deposit was paid: depositDue is the whole deposit, depositOwed what is left.
    const { wrapper } = await openOrder(asAdmin('aguardando_sinal', { depositDue: brl(64995), depositOwed: brl(30000), paid: brl(34995), balanceDue: brl(95000) }))

    await button(wrapper, 'Registrar pagamento').trigger('click')

    expect((field(wrapper, 'Tipo').element as HTMLSelectElement).value).toBe('deposit')
    expect((field(wrapper, 'Valor').element as HTMLInputElement).value).toBe('300,00')
    await field(wrapper, 'Tipo').setValue('balance')
    expect((field(wrapper, 'Valor').element as HTMLInputElement).value).toBe('950,00')
  })

  it('fills the balance when the type is Saldo', async () => {
    const { wrapper } = await openOrder(asAdmin('pronto', paid()))

    await button(wrapper, 'Registrar pagamento').trigger('click')
    expect((field(wrapper, 'Tipo').element as HTMLSelectElement).value).toBe('balance')
    expect((field(wrapper, 'Valor').element as HTMLInputElement).value).toBe('649,95')
    await field(wrapper, 'Tipo').setValue('deposit')
    expect((field(wrapper, 'Valor').element as HTMLInputElement).value).toBe('649,95')
  })

  it('asks for an amount above zero, and says when it passes what is due', async () => {
    const state = asAdmin()
    state.answers.payment = () => orderProblem('payment_exceeds_due')
    const { wrapper } = await openOrder(state)
    await button(wrapper, 'Registrar pagamento').trigger('click')

    await field(wrapper, 'Valor').setValue('0')
    await button(wrapper, 'Registrar').trigger('click')
    expect(wrapper.get('[role=dialog]').text()).toContain('Informe um valor maior que zero.')
    expect(state.calls.payment).toBeUndefined()

    await field(wrapper, 'Valor').setValue('9999,00')
    await button(wrapper, 'Registrar').trigger('click')
    await flushPromises()
    expect(plain(wrapper.get('[role=dialog] [role=alert]').text())).toBe('O valor passa do que falta pagar (R$ 1.299,90).')
  })

  it('keeps the idempotency key on retry', async () => {
    const state = asAdmin()
    let attempt = 0
    state.answers.payment = () => {
      attempt += 1
      return attempt === 1 ? new HttpResponse(null, { status: 500 }) : HttpResponse.json(state.order, { status: 201 })
    }
    const { wrapper } = await openOrder(state)
    await button(wrapper, 'Registrar pagamento').trigger('click')

    await button(wrapper, 'Registrar').trigger('click')
    await vi.waitFor(() => expect(wrapper.get('[role=dialog]').text()).toContain('O servidor não respondeu.'))
    await button(wrapper, 'Registrar').trigger('click')
    await vi.waitFor(() => expect(wrapper.find('[role=dialog]').exists()).toBe(false))

    expect(state.calls.payment).toHaveLength(2)
    expect(state.calls.payment?.[1]?.key).toBe(state.calls.payment?.[0]?.key)
  })

  it('makes a new key when the amount changed, and for the next payment after a success', async () => {
    const state = asAdmin()
    let attempt = 0
    state.answers.payment = () => {
      attempt += 1
      return attempt === 1 ? new HttpResponse(null, { status: 500 }) : HttpResponse.json(state.order, { status: 201 })
    }
    const { wrapper } = await openOrder(state)
    await button(wrapper, 'Registrar pagamento').trigger('click')
    await button(wrapper, 'Registrar').trigger('click')
    await vi.waitFor(() => expect(wrapper.get('[role=dialog]').text()).toContain('O servidor não respondeu.'))

    await field(wrapper, 'Valor').setValue('100,00')
    await button(wrapper, 'Registrar').trigger('click')
    await vi.waitFor(() => expect(wrapper.find('[role=dialog]').exists()).toBe(false))
    expect(state.calls.payment?.[1]?.key).not.toBe(state.calls.payment?.[0]?.key)

    await button(wrapper, 'Registrar pagamento').trigger('click')
    await button(wrapper, 'Registrar').trigger('click')
    await vi.waitFor(() => expect(state.calls.payment).toHaveLength(3))
    expect(state.calls.payment?.[2]?.key).not.toBe(state.calls.payment?.[1]?.key)
  })

  it('closes the dialog without recording on "Cancelar"', async () => {
    const state = asAdmin()
    const { wrapper } = await openOrder(state)
    await button(wrapper, 'Registrar pagamento').trigger('click')

    await button(wrapper, 'Cancelar').trigger('click')

    expect(wrapper.find('[role=dialog]').exists()).toBe(false)
    expect(state.calls.payment).toBeUndefined()
  })

  it('does not offer "Registrar pagamento" to a viewer', async () => {
    const { wrapper } = await openOrder(asAdmin(), 'viewer')

    expect(wrapper.findAll('button').map((b) => b.text())).not.toContain('Registrar pagamento')
  })
})

describe('voiding a payment', () => {
  const payment = {
    id: 'p1',
    kind: 'deposit' as const,
    amount: brl(64995),
    method: 'pix' as const,
    recordedBy: { id: 'u1', name: 'Ana Souza' },
    recordedAt: '2026-11-27T15:00:00Z',
  }
  const paid = { paid: brl(64995), depositPaid: true, payments: [payment] }

  it('is the owner\'s: "Anular" is not there for an admin, staff or a viewer', async () => {
    for (const role of ['admin', 'staff', 'viewer'] as const) {
      const { wrapper } = await openOrder(asAdmin('confirmado', paid), role)
      expect(wrapper.findAll('.payments button').map((b) => b.text()), role).not.toContain('Anular')
    }
  })

  it('asks for the reason and voids the payment', async () => {
    const state = asOwner('confirmado', paid)
    const { wrapper } = await openOrder(state, 'owner')

    await button(wrapper, 'Anular').trigger('click')
    expect(plain(wrapper.get('[role=dialog] h2').text())).toBe('Anular o pagamento de R$ 649,95?')
    await button(wrapper, 'Anular pagamento').trigger('click')
    expect(wrapper.get('[role=dialog]').text()).toContain('Informe o motivo.')
    expect(state.calls.void).toBeUndefined()

    await field(wrapper, 'Motivo').setValue('Valor errado')
    await button(wrapper, 'Anular pagamento').trigger('click')
    await flushPromises()

    expect(state.calls.void?.[0]?.body).toEqual({ reason: 'Valor errado', version: 1 })
    expect(wrapper.find('[role=dialog]').exists()).toBe(false)
    expect(wrapper.text()).toContain('Pagamento anulado.')
  })

  it('shows a refusal in the dialog', async () => {
    const state = asOwner('confirmado', paid)
    state.answers.void = () => orderProblem('order_closed')
    const { wrapper } = await openOrder(state, 'owner')
    await button(wrapper, 'Anular').trigger('click')
    await field(wrapper, 'Motivo').setValue('Erro')

    await button(wrapper, 'Anular pagamento').trigger('click')
    await flushPromises()

    expect(wrapper.get('[role=dialog] [role=alert]').text()).toBe('Este pedido já foi entregue ou cancelado.')
  })

  it('has no "Anular" on a payment that is already voided', async () => {
    const voided = { ...payment, voided: { by: { id: 'u', name: 'Ana' }, at: '2026-11-27T16:00:00Z', reason: 'x' } }
    const { wrapper } = await openOrder(asOwner('confirmado', { payments: [voided] }), 'owner')

    expect(wrapper.findAll('.payments button').map((b) => b.text())).not.toContain('Anular')
  })
})

describe('releasing without the deposit', () => {
  it('is the owner\'s, while the deposit is awaited', async () => {
    const owner = await openOrder(asOwner(), 'owner')
    expect(button(owner.wrapper, 'Liberar sem sinal').exists()).toBe(true)

    for (const role of ['admin', 'staff', 'viewer'] as const) {
      const { wrapper } = await openOrder(asAdmin(), role)
      expect(wrapper.findAll('button').map((b) => b.text()), role).not.toContain('Liberar sem sinal')
    }
  })

  it('is not offered once the deposit is paid or the order was released', async () => {
    const paid = await openOrder(asOwner('confirmado', { depositPaid: true }), 'owner')
    expect(paid.wrapper.findAll('button').map((b) => b.text())).not.toContain('Liberar sem sinal')

    const released = await openOrder(asOwner('aguardando_sinal', { released: true }), 'owner')
    expect(released.wrapper.findAll('button').map((b) => b.text())).not.toContain('Liberar sem sinal')
  })

  it('asks first, then releases', async () => {
    const state = asOwner()
    const { wrapper } = await openOrder(state, 'owner')

    await button(wrapper, 'Liberar sem sinal').trigger('click')
    expect(wrapper.get('[role=dialog]').text()).toContain('Liberar o pedido sem o sinal? As compras e a produção podem começar antes do pagamento.')
    await button(wrapper, 'Liberar pedido').trigger('click')
    await flushPromises()

    expect(state.calls.release?.[0]?.body).toEqual({ version: 1 })
    expect(wrapper.text()).toContain('Pedido liberado.')
    expect(wrapper.find('[role=dialog]').exists()).toBe(false)
  })
})

describe('"Alterar data"', () => {
  it('saves the date and the reason, and opens the wa.me link the API returned', async () => {
    const state = asAdmin()
    const { wrapper } = await openOrder(state, 'admin')

    await button(wrapper, 'Alterar data').trigger('click')
    expect((field(wrapper, 'Nova data de entrega').element as HTMLInputElement).value).toBe('2026-12-12')
    await field(wrapper, 'Nova data de entrega').setValue('2026-12-20')
    await field(wrapper, 'Motivo').setValue('Atraso do fornecedor')
    await button(wrapper, 'Salvar e avisar o cliente').trigger('click')
    await flushPromises()

    expect(state.calls.date?.[0]?.body).toEqual({ date: '2026-12-20', reason: 'Atraso do fornecedor', version: 1 })
    expect(window.open).toHaveBeenCalledWith(whatsapp, '_blank', 'noopener')
    expect(wrapper.text()).toContain('Data alterada. Envie a mensagem para avisar o cliente.')
    expect(wrapper.find('[role=dialog]').exists()).toBe(false)
  })

  it('keeps "Abrir o WhatsApp" with the notice, for a browser that blocked the automatic opening', async () => {
    const state = asAdmin()
    const { wrapper } = await openOrder(state, 'admin')
    await button(wrapper, 'Alterar data').trigger('click')
    await button(wrapper, 'Salvar e avisar o cliente').trigger('click')
    await flushPromises()
    vi.mocked(window.open).mockClear()

    await button(wrapper.get('.notice'), 'Abrir o WhatsApp').trigger('click')

    expect(window.open).toHaveBeenCalledOnce()
    expect(window.open).toHaveBeenCalledWith(whatsapp, '_blank', 'noopener')
  })

  it('takes the button away with the next action', async () => {
    const state = asAdmin()
    const { wrapper } = await openOrder(state, 'admin')
    await button(wrapper, 'Alterar data').trigger('click')
    await button(wrapper, 'Salvar e avisar o cliente').trigger('click')
    await flushPromises()
    expect(wrapper.findAll('button').map((b) => b.text())).toContain('Abrir o WhatsApp')

    await button(wrapper, 'Mover para Confirmado').trigger('click')
    await flushPromises()

    expect(wrapper.findAll('button').map((b) => b.text())).not.toContain('Abrir o WhatsApp')
  })

  it('leaves the reason out when it is empty', async () => {
    const state = asAdmin()
    const { wrapper } = await openOrder(state, 'admin')
    await button(wrapper, 'Alterar data').trigger('click')

    await button(wrapper, 'Salvar e avisar o cliente').trigger('click')
    await flushPromises()

    expect(state.calls.date?.[0]?.body).toEqual({ date: '2026-12-12', version: 1 })
  })

  it('says "Escolha uma data a partir de hoje." when the API refuses the date, and opens no link', async () => {
    const state = asAdmin()
    state.answers.date = () => orderProblem('invalid')
    const { wrapper } = await openOrder(state, 'admin')
    await button(wrapper, 'Alterar data').trigger('click')

    await button(wrapper, 'Salvar e avisar o cliente').trigger('click')
    await flushPromises()

    expect(wrapper.get('[role=dialog] [role=alert]').text()).toBe('Escolha uma data a partir de hoje.')
    expect(window.open).not.toHaveBeenCalled()
  })

  it('closes on "Cancelar"', async () => {
    const { wrapper } = await openOrder(asAdmin(), 'admin')
    await button(wrapper, 'Alterar data').trigger('click')

    await button(wrapper, 'Cancelar').trigger('click')

    expect(wrapper.find('[role=dialog]').exists()).toBe(false)
  })
})

describe('"Enviar link de acompanhamento"', () => {
  it('asks for a new link and opens the wa.me link the API returned', async () => {
    const state = asAdmin()
    const { wrapper } = await openOrder(state, 'staff')

    await button(wrapper, 'Enviar link de acompanhamento').trigger('click')
    await vi.waitFor(() => expect(window.open).toHaveBeenCalledWith('https://wa.me/5592991234567?text=link', '_blank', 'noopener'))

    expect(state.calls.tracking).toHaveLength(1)
    expect(wrapper.text()).toContain('Cada envio cria um link novo. O link anterior deixa de funcionar.')
  })

  it('says the link was made and keeps "Abrir o WhatsApp" with the new link', async () => {
    const state = asAdmin()
    const { wrapper } = await openOrder(state, 'staff')
    await button(wrapper, 'Enviar link de acompanhamento').trigger('click')
    await vi.waitFor(() => expect(wrapper.get('.notice').text()).toContain('Link de acompanhamento criado.'))
    vi.mocked(window.open).mockClear()

    await button(wrapper.get('.notice'), 'Abrir o WhatsApp').trigger('click')

    expect(window.open).toHaveBeenCalledWith('https://wa.me/5592991234567?text=link', '_blank', 'noopener')
  })

  // Making a link rotates the order's token, which is a write: the order's
  // version moves on, and the link's answer does not carry the order. The next
  // write must send the new version, or the person is told "Outra pessoa alterou este pedido".
  it('reads the order again, so the next write sends the version the link moved on to', async () => {
    const state = asAdmin()
    state.answers.tracking = () => {
      state.order = { ...state.order, version: 2 }
      return HttpResponse.json({ trackingUrl: 'https://altarpm.encomendo.com.br/p/novo', whatsappUrl: 'https://wa.me/5592991234567?text=link' })
    }
    state.answers.advance = () => HttpResponse.json(state.order)
    const { wrapper } = await openOrder(state, 'staff')
    await button(wrapper, 'Enviar link de acompanhamento').trigger('click')
    await vi.waitFor(() => expect(wrapper.get('.notice').text()).toContain('Link de acompanhamento criado.'))
    await flushPromises()

    await button(wrapper, 'Mover para Confirmado').trigger('click')
    await flushPromises()

    expect(state.calls.advance?.[0]?.body).toEqual({ to: 'confirmado', version: 2 })
    expect(wrapper.text()).not.toContain('Outra pessoa alterou este pedido')
  })

  it('shows a refusal and opens nothing', async () => {
    const state = asAdmin()
    state.answers.tracking = () => orderProblem('order_closed')
    const { wrapper } = await openOrder(state, 'staff')

    await button(wrapper, 'Enviar link de acompanhamento').trigger('click')
    await flushPromises()

    expect(wrapper.get('.alert').text()).toBe('Este pedido já foi entregue ou cancelado.')
    expect(window.open).not.toHaveBeenCalled()
  })

  it('is not offered to a viewer', async () => {
    const { wrapper } = await openOrder(asAdmin(), 'viewer')

    expect(wrapper.findAll('button').map((b) => b.text())).not.toContain('Enviar link de acompanhamento')
  })
})

describe('cancelling', () => {
  it('asks for the reason, then cancels and says so', async () => {
    const state = asAdmin()
    state.answers.cancel = () => {
      state.order = orderAsAdmin(encomendaMontagem, 'aguardando_sinal', { status: { key: 'cancelado', label: 'Cancelado', clientLabel: 'Cancelado', phase: 'cancelled' }, moves: [] })
      return HttpResponse.json(state.order)
    }
    const { wrapper } = await openOrder(state, 'admin')

    await button(wrapper, 'Cancelar pedido').trigger('click')
    expect(wrapper.get('[role=dialog] h2').text()).toBe('Cancelar o pedido P-0231?')
    await button(wrapper.get('[role=dialog]'), 'Cancelar pedido').trigger('click')
    expect(wrapper.get('[role=dialog]').text()).toContain('Informe o motivo.')
    expect(state.calls.cancel).toBeUndefined()

    await field(wrapper, 'Motivo').setValue('Cliente desistiu')
    await wrapper.get('[role=dialog] form').trigger('submit')
    await flushPromises()

    expect(state.calls.cancel?.[0]?.body).toEqual({ reason: 'Cliente desistiu', version: 1 })
    expect(wrapper.text()).toContain('Pedido cancelado.')
    expect(wrapper.get('.order .header .enc-status-badge').text()).toBe('Cancelado')
    expect(wrapper.find('.advance').exists()).toBe(false)
  })

  it('says what stays when the order was paid', async () => {
    const state = asAdmin('confirmado', { paid: brl(64995) })
    const { wrapper } = await openOrder(state, 'admin')

    await button(wrapper, 'Cancelar pedido').trigger('click')

    expect(wrapper.get('[role=dialog]').text()).toContain('Os pagamentos registrados continuam no pedido. A devolução é feita fora do Encomendo.')
  })

  it('does not say it for an order with nothing paid', async () => {
    const { wrapper } = await openOrder(asAdmin(), 'admin')

    await button(wrapper, 'Cancelar pedido').trigger('click')

    expect(wrapper.get('[role=dialog]').text()).not.toContain('Os pagamentos registrados continuam')
  })

  it('is for the owner and an admin, not for staff or a viewer', async () => {
    for (const [role, shown] of [['owner', true], ['admin', true], ['staff', false], ['viewer', false]] as const) {
      const { wrapper } = await openOrder(asAdmin(), role)
      expect(wrapper.findAll('button').map((b) => b.text()).includes('Cancelar pedido'), role).toBe(shown)
    }
  })

  it('closes with "Voltar" without cancelling', async () => {
    const state = asAdmin()
    const { wrapper } = await openOrder(state, 'admin')
    await button(wrapper, 'Cancelar pedido').trigger('click')

    await button(wrapper, 'Voltar').trigger('click')

    expect(wrapper.find('[role=dialog]').exists()).toBe(false)
    expect(state.calls.cancel).toBeUndefined()
  })
})

describe('"Histórico"', () => {
  const by = { id: 'u1', name: 'Ana' }
  const entries: HistoryEntry[] = [
    { kind: 'cancelled', at: '2026-11-27T20:00:00Z', by, reason: 'Cliente desistiu' },
    { kind: 'tracking_link', at: '2026-11-27T19:00:00Z', by },
    { kind: 'promised_date', at: '2026-11-27T18:30:00Z', by, itemId: boughtItem.id, itemName: 'Banco de couro Scrambler', newDate: '2026-12-09', ref: 'C-0012' },
    { kind: 'changed', at: '2026-11-27T18:00:00Z', by, customerApproved: true },
    { kind: 'date', at: '2026-11-27T17:00:00Z', by, oldDate: '2026-12-12', newDate: '2026-12-20' },
    { kind: 'payment', at: '2026-11-27T16:00:00Z', by, paymentKind: 'deposit', paymentMethod: 'pix', amount: brl(30000) },
    { kind: 'status', at: '2026-11-27T15:00:00Z', by, to: { key: 'em_montagem', label: 'Em montagem', clientLabel: 'Montando', phase: 'assembling' } },
    { kind: 'item', at: '2026-11-27T14:00:00Z', by, itemId: boughtItem.id, itemName: 'Banco de couro Scrambler', itemTo: 'received', check: 'defective' },
    { kind: 'payment_voided', at: '2026-11-27T13:00:00Z', by, amount: brl(30000), reason: 'Erro' },
    { kind: 'released', at: '2026-11-27T12:30:00Z', by },
    { kind: 'backup_supplier', at: '2026-11-27T12:15:00Z', by, itemName: 'Banco de couro Scrambler' },
    { kind: 'created', at: '2026-11-27T12:00:00Z', by: null },
  ]

  it('writes each entry as "{quem} {o quê}" with its date and time, newest first', async () => {
    const state = asAdmin()
    state.history = entries
    const { wrapper } = await openOrder(state)
    await vi.waitFor(() => expect(wrapper.find('.history li').exists()).toBe(true))

    const lines = wrapper.findAll('.history li span').map((span) => plain(span.text()))
    expect(lines).toEqual([
      'Ana cancelou: Cliente desistiu',
      'Ana enviou o link de acompanhamento',
      'Ana informou o prazo do fornecedor de Banco de couro Scrambler: 09/12/2026',
      'Ana alterou o pedido (cliente aprovou)',
      'Ana alterou a entrega de 12/12/2026 para 20/12/2026',
      'Ana registrou o sinal de R$ 300,00 (Pix)',
      'Ana moveu para Em montagem',
      'Ana recebeu Banco de couro Scrambler com defeito',
      'Ana anulou o pagamento de R$ 300,00: Erro',
      'Ana liberou o pedido sem o sinal',
      'Ana usou o fornecedor reserva em Banco de couro Scrambler',
      'O sistema criou o pedido',
    ])
    expect(wrapper.get('.history li time').attributes('datetime')).toBe('2026-11-27T20:00:00Z')
  })

  it("writes the purchasing entries as the design document's Histórico does", async () => {
    const painel = item({ id: 'painel', name: 'Painel', sourcingMode: 'in_house', status: 'ready', nextMoves: ['installed'] })
    const state = newOrderState(withItems(orderAsAdmin(), [boughtItem, painel]))
    const move = (itemTo: NonNullable<HistoryEntry['itemTo']>, extra: Partial<HistoryEntry> = {}): HistoryEntry => ({
      kind: 'item',
      at: '2026-11-27T14:00:00Z',
      by,
      itemId: boughtItem.id,
      itemName: 'Banco de couro Scrambler',
      itemTo,
      ...extra,
    })
    state.history = [
      move('received', { check: 'defective', reason: 'Veio riscado', ref: 'C-0012' }),
      move('received', { check: 'ok', ref: 'C-0012' }),
      move('received', { check: 'ok' }),
      move('to_buy', { itemFrom: 'sent_to_supplier', ref: 'C-0012' }),
      move('in_transit', { ref: 'C-0012' }),
      move('sent_to_supplier', { ref: 'C-0012' }),
      move('installed'),
      { ...move('ready', { check: 'defective' }), itemId: 'painel', itemName: 'Painel' },
    ]
    const { wrapper } = await openOrder(state)
    await vi.waitFor(() => expect(wrapper.find('.history li').exists()).toBe(true))

    expect(wrapper.findAll('.history li span').map((span) => plain(span.text()))).toEqual([
      'Ana recebeu Banco de couro Scrambler com defeito: Veio riscado',
      'Ana recebeu Banco de couro Scrambler (tudo certo)',
      'Ana recebeu Banco de couro Scrambler (tudo certo)',
      'Ana desfez o envio de Banco de couro Scrambler (C-0012)',
      'Ana marcou Banco de couro Scrambler como em trânsito',
      'Ana enviou Banco de couro Scrambler ao fornecedor (C-0012)',
      'Ana marcou Banco de couro Scrambler como instalada',
      'Ana marcou Painel como pronto (com defeito)',
    ])
  })

  it('reads the next page on "Carregar mais"', async () => {
    const cursors: (string | null)[] = []
    signedInAs('owner')
    serveOrder(server, asAdmin())
    server.use(
      http.get('*/api/v1/admin/orders/:id/history', ({ request }) => {
        const cursor = new URL(request.url).searchParams.get('cursor')
        cursors.push(cursor)
        return cursor === null ? HttpResponse.json({ items: [entries[0]], nextCursor: 'h2' }) : HttpResponse.json({ items: [entries[1]] })
      }),
    )
    const { wrapper } = await openApp(`/pedidos/${orderId}`)
    await vi.waitFor(() => expect(wrapper.text()).toContain('Ana cancelou'))

    await button(wrapper.get('.history'), 'Carregar mais').trigger('click')
    await vi.waitFor(() => expect(wrapper.text()).toContain('Ana enviou o link de acompanhamento'))

    expect(cursors).toEqual([null, 'h2'])
    expect(wrapper.findAll('.history li')).toHaveLength(2)
  })

  it('is read again after an action', async () => {
    const state = asAdmin()
    state.history = []
    const { wrapper } = await openOrder(state)
    state.history = [{ kind: 'tracking_link', at: '2026-11-27T19:00:00Z', by: { id: 'u1', name: 'Ana' } }]

    await button(wrapper, 'Enviar link de acompanhamento').trigger('click')

    await vi.waitFor(() => expect(wrapper.get('.history').text()).toContain('Ana enviou o link de acompanhamento'))
  })
})

describe('another person changed the order', () => {
  it('says so, with "Recarregar", and reads the order again', async () => {
    const state = asAdmin()
    state.answers.advance = () => orderProblem('version_conflict')
    const { wrapper } = await openOrder(state)

    await button(wrapper, 'Mover para Confirmado').trigger('click')
    await flushPromises()

    expect(wrapper.get('.alert').text()).toContain('Outra pessoa alterou este pedido. Recarregue para ver a versão atual.')
    state.order = orderAsAdmin(encomendaMontagem, 'confirmado', { version: 2 })
    await button(wrapper, 'Recarregar').trigger('click')
    await vi.waitFor(() => expect(wrapper.get('.order .header .enc-status-badge').text()).toBe('Confirmado'))

    expect(wrapper.find('.alert').exists()).toBe(false)
  })

  it('sends the version it read with every write', async () => {
    const state = asAdmin('aguardando_sinal', { version: 7 })
    const { wrapper } = await openOrder(state, 'owner')

    await button(wrapper, 'Mover para Confirmado').trigger('click')
    await flushPromises()
    await button(wrapper, 'Enviar link de acompanhamento').trigger('click')
    await flushPromises()

    expect(state.calls.advance?.[0]?.body.version).toBe(7)
  })
})

describe('a conflict met inside a dialog', () => {
  it('closes the dialog and offers "Recarregar", since a retry would send the same version', async () => {
    const state = asAdmin()
    state.answers.payment = () => orderProblem('version_conflict')
    const { wrapper } = await openOrder(state)
    await button(wrapper, 'Registrar pagamento').trigger('click')

    await button(wrapper, 'Registrar').trigger('click')
    await flushPromises()

    expect(wrapper.find('[role=dialog]').exists()).toBe(false)
    expect(wrapper.get('.alert').text()).toContain('Outra pessoa alterou este pedido. Recarregue para ver a versão atual.')
    expect(button(wrapper, 'Recarregar').exists()).toBe(true)
  })
})

describe('owner and admin', () => {
  it('never shows cost or margin on an admin\'s order, anywhere on the screen', async () => {
    const { wrapper } = await openOrder(asAdmin(), 'admin')

    expect(wrapper.text()).not.toMatch(/Custo|Margem/)
  })

  it('shows them on the owner\'s order', async () => {
    const { wrapper } = await openOrder(asOwner(), 'owner')

    expect(wrapper.text()).toMatch(/Custo/)
    expect(wrapper.text()).toMatch(/Margem/)
  })
})

describe('"Alterar pedido" on the order screen', () => {
  it('links to the change form for a role that writes a free order', async () => {
    const { wrapper } = await openOrder(asAdmin(), 'staff')

    expect(link(wrapper, 'Alterar pedido').attributes('href')).toBe(`/pedidos/${orderId}/alterar`)
  })

  it('is not offered to staff once the order has a discount or a payment', async () => {
    const discounted = await openOrder(asAdmin('aguardando_sinal', { discount: brl(1000) }), 'staff')
    expect(discounted.wrapper.findAll('a').map((a) => a.text())).not.toContain('Alterar pedido')

    const paid = await openOrder(asAdmin('confirmado', { editable: 'approval' }), 'staff')
    expect(paid.wrapper.findAll('a').map((a) => a.text())).not.toContain('Alterar pedido')

    const admin = await openOrder(asAdmin('confirmado', { editable: 'approval', discount: brl(1000) }), 'admin')
    expect(admin.wrapper.findAll('a').map((a) => a.text())).toContain('Alterar pedido')
  })

  it('is not offered when the order can no longer be changed', async () => {
    const { wrapper } = await openOrder(asAdmin('em_montagem', { editable: 'closed' }), 'admin')

    expect(wrapper.findAll('a').map((a) => a.text())).not.toContain('Alterar pedido')
  })
})

describe('the phone layout', () => {
  // The layout itself is CSS (the Playwright phone project checks 390 px); these are the structures it relies on.
  it('turns "Itens" and "Pagamentos" into cards with labelled cells', async () => {
    const state = asAdmin('confirmado', {
      payments: [{ id: 'p1', kind: 'deposit', amount: brl(100), method: 'pix', recordedBy: { id: 'u', name: 'Ana' }, recordedAt: '2026-11-27T15:00:00Z' }],
    })
    const { wrapper } = await openOrder(state)

    for (const table of wrapper.findAll('table')) expect(table.classes()).toContain('enc-cards')
    expect(wrapper.findAll('.items tbody tr')[0]?.findAll('td').map((cell) => cell.attributes('data-label'))).toEqual(['Origem', 'Situação', 'Previsto', 'Ações'])
    expect(wrapper.findAll('.payments tbody tr')[0]?.findAll('td').map((cell) => cell.attributes('data-label'))).toEqual(['Data', 'Tipo', 'Forma', 'Valor', 'Registrado por', undefined])
    expect(wrapper.get('.items tbody tr th').text()).toContain('Banco de couro Scrambler')
    expect(wrapper.get('.items tbody td.enc-cards__actions').element).toBeTruthy()
  })

  it('puts the buttons of every dialog in the sheet footer', async () => {
    const { wrapper } = await openOrder(asOwner())

    const dialogs: [string, string[]][] = [
      ['Registrar pagamento', ['Registrar', 'Cancelar']],
      ['Liberar sem sinal', ['Liberar pedido', 'Voltar']],
      ['Alterar data', ['Salvar e avisar o cliente', 'Cancelar']],
      ['Cancelar pedido', ['Cancelar pedido', 'Voltar']],
    ]
    for (const [opener, expected] of dialogs) {
      await button(wrapper, opener).trigger('click')
      expect(wrapper.get('[role=dialog] .enc-sheet-actions').findAll('button').map((b) => b.text()), opener).toEqual(expected)
      await button(wrapper.get('[role=dialog]'), expected[expected.length - 1] ?? '').trigger('click')
      expect(wrapper.find('[role=dialog]').exists()).toBe(false)
    }
  })

  it('keeps the timeline rows as a list of items with their dates', async () => {
    const { wrapper } = await openOrder(asAdmin())

    const row = wrapper.get('.timeline .row')
    expect(row.get('.name').text()).toBe('Banco de couro Scrambler')
    expect(row.get('.date').text()).toBe('Previsto: 05/12/2026')
  })
})
