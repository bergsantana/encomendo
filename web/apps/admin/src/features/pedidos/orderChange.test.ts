import type { ChangedOrder, Order, Permission } from '@encomendo/api'
import { enableAutoUnmount, flushPromises } from '@vue/test-utils'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'

import { loadPage } from '../../pageLoad'
import { button, closeApps, field, link, openApp } from '../../testing/app'
import { authSession } from '../../testing/fixtures'
import { brl, encomendaMontagem, joao, orderAsAdmin, orderProblem } from '../../testing/orders'
import { bancoId, corOption, guidaoAltoId, guidaoId, kit, kitId, newOrderState, type OrderState, quoteFor, serveCatalog, serveOrder } from './testkit'

vi.mock('../../pageLoad', () => ({ loadPage: vi.fn() }))

const server = setupServer()
beforeAll(() => server.listen({ onUnhandledFrame: 'error' }))
afterEach(() => {
  server.resetHandlers()
  vi.mocked(loadPage).mockClear()
  vi.restoreAllMocks()
})
afterAll(() => server.close())
enableAutoUnmount(afterEach)
afterEach(closeApps)

type Role = 'owner' | 'admin' | 'staff'
const rights: Record<Role, Permission[]> = {
  owner: ['order:read', 'order:write', 'order:manage', 'order:override', 'customer:write'],
  admin: ['order:read', 'order:write', 'order:manage', 'customer:write'],
  staff: ['order:read', 'order:write'],
}
const signedInAs = (role: Role) => server.use(http.post('*/auth/refresh', () => HttpResponse.json(authSession({ role, permissions: rights[role] }))))

const plain = (text: string) => text.replace(/\u00a0/g, ' ')
const preto = corOption.choices[0]
const chosenCor = { optionId: corOption.id, choiceId: preto?.id ?? '', name: 'Cor', label: 'Preto', extraPrice: brl(0), extraDays: 0 }
const whatsapp = 'https://wa.me/5592991234567?text=alterado'

/** looseOrder is an order with the banco as one loose line, its required colour chosen. */
function looseOrder(statusKey = 'aguardando_sinal', overrides: Partial<Order> = {}): Order {
  const base = orderAsAdmin(encomendaMontagem, statusKey, overrides)
  const [first] = base.items
  return { ...base, items: first === undefined ? [] : [{ ...first, options: [chosenCor] }] }
}

/** kitOrder is an order with the kit, twice, its guidão swapped for the taller one. */
function kitOrder(overrides: Partial<Order> = {}): Order {
  const base = orderAsAdmin(encomendaMontagem, 'aguardando_sinal', overrides)
  const template = base.items[0]
  if (template === undefined) throw new Error('fixture')
  const lineId = base.lines[0]?.id ?? ''
  return {
    ...base,
    lines: [
      {
        id: lineId,
        kit: { id: kitId, name: kit.name, swaps: [{ removedItemId: guidaoId, removedName: 'Guidão baixo', addedItemId: guidaoAltoId, addedName: 'Guidão alto' }] },
        price: brl(140000),
        itemIds: ['i1', 'i2'],
      },
    ],
    items: [
      { ...template, id: 'i1', lineId, catalogItemId: bancoId, name: 'Banco de couro Scrambler', quantity: 2, options: [chosenCor] },
      { ...template, id: 'i2', lineId, catalogItemId: guidaoAltoId, name: 'Guidão alto', quantity: 2, options: [] },
    ],
  }
}

interface Calls {
  quotes: Record<string, unknown>[]
  changes: Record<string, unknown>[]
}

function serve(state: OrderState, change?: () => Response | Promise<Response>): Calls {
  const calls: Calls = { quotes: [], changes: [] }
  serveCatalog(server)
  serveOrder(server, state)
  server.use(
    http.get('*/api/v1/admin/customers/:id', () => HttpResponse.json(joao)),
    http.post('*/api/v1/admin/orders/quote', async ({ request }) => {
      calls.quotes.push((await request.json()) as Record<string, unknown>)
      return HttpResponse.json(quoteFor())
    }),
    http.put('*/api/v1/admin/orders/:id', async ({ request }) => {
      calls.changes.push((await request.json()) as Record<string, unknown>)
      if (change !== undefined) return change()
      const answer: ChangedOrder = {
        order: state.order,
        change: { paid: false, oldTotal: brl(129990), newTotal: brl(123400), oldDeliveryDate: '2026-12-12', newDeliveryDate: '2026-12-20', refundDue: brl(0) },
      }
      return HttpResponse.json(answer)
    }),
  )
  return calls
}

async function openChange(state: OrderState, role: Role = 'admin', change?: () => Response | Promise<Response>) {
  signedInAs(role)
  const calls = serve(state, change)
  vi.spyOn(window, 'open').mockReturnValue(null)
  const app = await openApp(`/pedidos/${state.order.id}/alterar`)
  await vi.waitFor(() => expect(app.wrapper.get('h1').text()).toBe('Alterar pedido P-0231'))
  return { ...app, calls }
}

beforeEach(() => {
  signedInAs('admin')
})

describe('"Alterar pedido"', () => {
  it('opens with the order\'s customer fixed, its lines and its terms in the form', async () => {
    const { wrapper, calls } = await openChange(newOrderState(looseOrder()))
    await vi.waitFor(() => expect(wrapper.find('.line').exists()).toBe(true))

    expect(wrapper.get('.chosen').text()).toContain('João Pereira')
    expect(wrapper.text()).not.toContain('Trocar cliente')
    expect(wrapper.get('.line h3').text()).toBe('Banco de couro Scrambler')
    expect((field(wrapper, 'Cor').element as HTMLSelectElement).value).toBe(preto?.id)
    expect((field(wrapper, 'Quantidade').element as HTMLInputElement).value).toBe('1')
    expect((field(wrapper, 'Retirada na loja').element as HTMLInputElement).checked).toBe(true)
    expect(wrapper.get('.actions').findAll('button').map((b) => b.text())).toEqual(['Salvar alterações', 'Cancelar'])
    await vi.waitFor(() => expect(calls.quotes).toHaveLength(1))
  })

  it('asks the quote about this order, so dates count from its release', async () => {
    const { calls } = await openChange(newOrderState(looseOrder()))

    await vi.waitFor(() => expect(calls.quotes).toHaveLength(1))

    expect(calls.quotes[0]).toEqual({
      orderId: looseOrder().id,
      lines: [{ itemId: bancoId, quantity: 1, options: [{ itemId: bancoId, optionId: corOption.id, choiceId: preto?.id }] }],
      terms: { handover: 'pickup', depositPercent: 50, assemblyDays: 3, safetyMarginDays: 2 },
    })
  })

  it('rebuilds a kit line with its swap, its options and its quantity', async () => {
    const { wrapper, calls } = await openChange(newOrderState(kitOrder()))
    await vi.waitFor(() => expect(calls.quotes).toHaveLength(1))

    expect(wrapper.get('.line h3').text()).toBe('Kit Scrambler')
    expect(wrapper.text()).toContain('Guidão alto no lugar de Guidão baixo')
    expect((field(wrapper, 'Quantidade').element as HTMLInputElement).value).toBe('2')
    expect(calls.quotes[0]?.lines).toEqual([
      {
        kitId,
        quantity: 2,
        swaps: [{ itemId: guidaoId, withItemId: guidaoAltoId }],
        options: [{ itemId: bancoId, optionId: corOption.id, choiceId: preto?.id }],
      },
    ])
  })

  it('shows what changes, from the order and the quote: total and delivery date', async () => {
    const { wrapper } = await openChange(newOrderState(looseOrder()))

    await vi.waitFor(() => expect(wrapper.find('.change').exists()).toBe(true))

    const text = plain(wrapper.get('.change').text())
    expect(text).toContain('TotalR$ 1.299,90 → R$ 1.234,00')
    expect(text).toContain('Entrega prevista12/12/2026 → 20/12/2026')
    expect(text).not.toContain('Valor a devolver')
  })

  it('saves the change with the version read and says "Pedido alterado."', async () => {
    const { wrapper, calls } = await openChange(newOrderState(looseOrder('aguardando_sinal', { version: 4 })))
    await vi.waitFor(() => expect(calls.quotes).toHaveLength(1))
    await field(wrapper, 'Quantidade').setValue('3')

    await button(wrapper, 'Salvar alterações').trigger('click')
    await vi.waitFor(() => expect(wrapper.text()).toContain('Pedido alterado.'))

    expect(calls.changes).toEqual([
      {
        lines: [{ itemId: bancoId, quantity: 3, options: [{ itemId: bancoId, optionId: corOption.id, choiceId: preto?.id }] }],
        terms: { handover: 'pickup', depositPercent: 50, assemblyDays: 3, safetyMarginDays: 2 },
        customerApproved: false,
        version: 4,
      },
    ])
    expect(plain(wrapper.get('.change').text())).toContain('TotalR$ 1.299,90 → R$ 1.234,00')
    expect(link(wrapper, 'Abrir pedido').attributes('href')).toBe(`/pedidos/${looseOrder().id}`)
    // Nothing was paid, so there is no message to send.
    expect(wrapper.findAll('button').map((b) => b.text())).not.toContain('Avisar o cliente pelo WhatsApp')
  })

  it('does not ask for the customer\'s approval before any payment', async () => {
    const { wrapper } = await openChange(newOrderState(looseOrder()))

    expect(wrapper.text()).not.toContain('O cliente aprovou a mudança')
    expect(wrapper.text()).not.toContain('Este pedido já tem pagamento.')
  })
})

describe('changing a paid order', () => {
  const paid = (extra: Partial<Order> = {}) => looseOrder('confirmado', { editable: 'approval', paid: brl(64995), depositPaid: true, version: 3, ...extra })
  const refund: ChangedOrder = {
    order: paid(),
    change: { paid: true, oldTotal: brl(129990), newTotal: brl(50000), oldDeliveryDate: '2026-12-12', newDeliveryDate: '2026-12-12', refundDue: brl(14995) },
    whatsappUrl: whatsapp,
  }

  it('warns, and asks for the box "O cliente aprovou a mudança"', async () => {
    const { wrapper } = await openChange(newOrderState(paid()))

    expect(wrapper.get('.notice').text()).toBe('Este pedido já tem pagamento. Combine a mudança com o cliente antes de salvar.')
    expect(field(wrapper, 'O cliente aprovou a mudança').element).toBeTruthy()
  })

  it('asks for the confirmation when saving without it, and sends nothing', async () => {
    const { wrapper, calls } = await openChange(newOrderState(paid()))
    await vi.waitFor(() => expect(calls.quotes).toHaveLength(1))

    await button(wrapper, 'Salvar alterações').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('Confirme que o cliente aprovou a mudança.')
    expect(calls.changes).toEqual([])
  })

  it('asks for approval in the API\'s words when the server refuses without it', async () => {
    const { wrapper, calls } = await openChange(newOrderState(paid()), 'admin', () => orderProblem('approval_required'))
    await vi.waitFor(() => expect(calls.quotes).toHaveLength(1))
    await field(wrapper, 'O cliente aprovou a mudança').setValue(true)

    await button(wrapper, 'Salvar alterações').trigger('click')
    await flushPromises()

    expect(wrapper.get('.alert').text()).toBe('Confirme que o cliente aprovou a mudança.')
  })

  it('sends the approval, then shows what changed, the refund and "Avisar o cliente pelo WhatsApp"', async () => {
    const { wrapper, calls } = await openChange(newOrderState(paid()), 'admin', () => HttpResponse.json(refund))
    await vi.waitFor(() => expect(calls.quotes).toHaveLength(1))
    await field(wrapper, 'O cliente aprovou a mudança').setValue(true)

    await button(wrapper, 'Salvar alterações').trigger('click')
    await vi.waitFor(() => expect(wrapper.text()).toContain('Pedido alterado.'))

    expect(calls.changes[0]?.customerApproved).toBe(true)
    expect(calls.changes[0]?.version).toBe(3)
    const summary = plain(wrapper.get('.change').text())
    expect(summary).toContain('TotalR$ 1.299,90 → R$ 500,00')
    expect(summary).toContain('Valor a devolverR$ 149,95')
    await button(wrapper, 'Avisar o cliente pelo WhatsApp').trigger('click')
    expect(window.open).toHaveBeenCalledWith(whatsapp, '_blank', 'noopener')
  })

  it('is for the owner and admins: staff see why they cannot, and the form is not there', async () => {
    const { wrapper } = await openChange(newOrderState(paid()), 'staff')

    expect(wrapper.get('[role=alert]').text()).toBe('Só o Dono ou um Admin altera um pedido com pagamento ou desconto.')
    expect(wrapper.find('form').exists()).toBe(false)
    expect(link(wrapper, 'Abrir pedido').attributes('href')).toBe(`/pedidos/${paid().id}`)
  })

  it('says so past assembling, with the niche word', async () => {
    const { wrapper } = await openChange(newOrderState(looseOrder('em_montagem', { editable: 'closed' })))

    expect(wrapper.get('[role=alert]').text()).toBe('Este pedido já está em Montagem e não pode mais ser alterado.')
    expect(wrapper.find('form').exists()).toBe(false)
  })

  it('says the order is closed once delivered', async () => {
    const { wrapper } = await openChange(newOrderState(looseOrder('entregue', { editable: 'closed' })))

    expect(wrapper.get('[role=alert]').text()).toBe('Este pedido já foi entregue ou cancelado.')
  })
})

describe('the discount lock', () => {
  const discounted = () => looseOrder('aguardando_sinal', { discount: brl(5000) })

  it('keeps staff out of a discounted order, even before any payment', async () => {
    const { wrapper } = await openChange(newOrderState(discounted()), 'staff')

    expect(wrapper.get('[role=alert]').text()).toBe('Só o Dono ou um Admin altera um pedido com pagamento ou desconto.')
    expect(wrapper.find('form').exists()).toBe(false)
  })

  it('lets an admin change it, with the discount in the form and in the request', async () => {
    const { wrapper, calls } = await openChange(newOrderState(discounted()), 'admin')
    await vi.waitFor(() => expect(calls.quotes).toHaveLength(1))

    expect((field(wrapper, 'Desconto').element as HTMLInputElement).value).toBe('50,00')
    expect(calls.quotes[0]?.terms).toMatchObject({ discountCents: 5000 })
  })

  it('lets staff change a free order, sending no discount, deposit or days', async () => {
    const { wrapper, calls } = await openChange(newOrderState(looseOrder()), 'staff')
    await vi.waitFor(() => expect(calls.quotes).toHaveLength(1))

    expect(wrapper.findAll('label').map((label) => label.text())).not.toContain('Desconto')
    expect(calls.quotes[0]?.terms).toEqual({ handover: 'pickup' })
    await button(wrapper, 'Salvar alterações').trigger('click')
    await vi.waitFor(() => expect(wrapper.text()).toContain('Pedido alterado.'))
    expect(calls.changes[0]?.terms).toEqual({ handover: 'pickup' })
  })
})

describe('refusals of a change', () => {
  it('says another person changed the order, and "Recarregar" reads it again', async () => {
    const state = newOrderState(looseOrder())
    let tries = 0
    const { wrapper, calls } = await openChange(state, 'admin', () => {
      tries += 1
      return tries === 1
        ? orderProblem('version_conflict')
        : HttpResponse.json({ order: state.order, change: { paid: false, oldTotal: brl(1), newTotal: brl(1), oldDeliveryDate: '2026-12-12', newDeliveryDate: '2026-12-12', refundDue: brl(0) } })
    })
    await vi.waitFor(() => expect(calls.quotes).toHaveLength(1))

    await button(wrapper, 'Salvar alterações').trigger('click')
    await flushPromises()
    expect(wrapper.get('.alert').text()).toContain('Outra pessoa alterou este pedido. Recarregue para ver a versão atual.')

    state.order = looseOrder('aguardando_sinal', { version: 9 })
    await button(wrapper, 'Recarregar').trigger('click')
    await vi.waitFor(() => expect(wrapper.findAll('button').map((b) => b.text())).toContain('Salvar alterações'))
    await button(wrapper, 'Salvar alterações').trigger('click')
    await flushPromises()

    await vi.waitFor(() => expect(wrapper.text()).toContain('Pedido alterado.'))
    expect(calls.changes.at(-1)?.version).toBe(9)
  })

  it('says the order can no longer be changed when it entered assembly meanwhile', async () => {
    const { wrapper, calls } = await openChange(newOrderState(looseOrder()), 'admin', () => orderProblem('order_not_editable'))
    await vi.waitFor(() => expect(calls.quotes).toHaveLength(1))

    await button(wrapper, 'Salvar alterações').trigger('click')
    await flushPromises()

    expect(wrapper.get('.alert').text()).toBe('Este pedido já está em montagem e não pode mais ser alterado.')
  })

  it('names an archived item', async () => {
    const { wrapper, calls } = await openChange(newOrderState(looseOrder()), 'admin', () =>
      orderProblem('item_unavailable', { references: [{ id: bancoId, name: 'Banco de couro Scrambler' }] }),
    )
    await vi.waitFor(() => expect(calls.quotes).toHaveLength(1))

    await button(wrapper, 'Salvar alterações').trigger('click')
    await flushPromises()

    expect(wrapper.get('.alert').text()).toBe('Banco de couro Scrambler está arquivado e não pode entrar no pedido.')
  })

  it('says "Pedido não encontrado." for an order that is not there', async () => {
    signedInAs('admin')
    serve(newOrderState(looseOrder()))
    server.use(http.get('*/api/v1/admin/orders/:id', () => orderProblem('not_found')))

    const { wrapper } = await openApp('/pedidos/inexistente/alterar')
    await vi.waitFor(() => expect(wrapper.text()).toContain('Pedido não encontrado.'))

    expect(button(wrapper, 'Tentar de novo').exists()).toBe(true)
  })
})

describe('leaving', () => {
  it('goes back to the order on "Cancelar" with nothing edited', async () => {
    const { wrapper, router } = await openChange(newOrderState(looseOrder()))

    await button(wrapper, 'Cancelar').trigger('click')

    await vi.waitFor(() => expect(router.currentRoute.value.name).toBe('order'))
  })

  it('asks "Descartar as alterações?" after an edit', async () => {
    const { wrapper, router } = await openChange(newOrderState(looseOrder()))
    await field(wrapper, 'Quantidade').setValue('5')

    await button(wrapper, 'Cancelar').trigger('click')
    await vi.waitFor(() => expect(wrapper.text()).toContain('Descartar as alterações?'))

    expect(router.currentRoute.value.name).toBe('change-order')
  })
})

describe('the route', () => {
  it('needs order:write: a viewer is sent to Hoje', async () => {
    server.use(http.post('*/auth/refresh', () => HttpResponse.json(authSession({ role: 'viewer', permissions: ['order:read'] }))))
    serve(newOrderState(looseOrder()))

    const { router } = await openApp(`/pedidos/${looseOrder().id}/alterar`)

    expect(router.currentRoute.value.name).toBe('hoje')
  })
})

describe('the phone layout', () => {
  it('keeps the approval as an enc-check and the actions in a wrapping row', async () => {
    const { wrapper } = await openChange(newOrderState(looseOrder('confirmado', { editable: 'approval', paid: brl(100), depositPaid: true })))

    expect(wrapper.get('.approval').classes()).toContain('enc-check')
    expect(wrapper.get('.approval input').element.closest('label')).not.toBeNull()
    expect(wrapper.get('.actions').element).toBeTruthy()
  })
})
