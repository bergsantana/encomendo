import type { Permission, SupplierOrder } from '@encomendo/api'
import { enableAutoUnmount, flushPromises } from '@vue/test-utils'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'

import { button, closeApps, field, link, openApp } from '../../testing/app'
import { authSession, problem } from '../../testing/fixtures'
import { brl } from '../../testing/orders'
import { links, orderIds, purchaseLine, serve, supplierOrderAnswer, supplierOrderId } from './testkit'

const server = setupServer()
beforeAll(() => server.listen({ onUnhandledFrame: 'error' }))
afterEach(() => {
  server.resetHandlers()
  vi.restoreAllMocks()
})
afterAll(() => server.close())
enableAutoUnmount(afterEach)
afterEach(closeApps)

const buyer: Permission[] = ['order:read', 'catalog:read', 'catalog:write', 'purchase:write', 'settings:read']

const signedInAs = (role: 'owner' | 'admin', permissions: Permission[] = buyer) =>
  server.use(http.post('*/auth/refresh', () => HttpResponse.json(authSession({ role, permissions }))))

const plain = (text: string) => text.replace(/\u00a0/g, ' ')
const path = `*/api/v1/admin/purchasing/supplier-orders/${supplierOrderId}`

/** Writes are what the screen sent to the pedido de compra, in order. */
interface Sent {
  url: string
  body: unknown
}

/** openOrder opens the screen on a pedido de compra; `owner` is the owner's view of it. */
async function openOrder(order: SupplierOrder = supplierOrderAnswer(), role: 'owner' | 'admin' = 'admin', permissions: Permission[] = buyer) {
  signedInAs(role, permissions)
  server.use(http.get(path, () => HttpResponse.json(order)), ...serve().handlers)
  const app = await openApp(`/compras/pedidos/${supplierOrderId}`)
  await vi.waitFor(() => expect(app.wrapper.text()).toContain('Itens'))
  return app
}

/** answerWrites answers the writes on the pedido de compra and records them. */
function answerWrites(answer: (url: string, body: unknown) => Response | SupplierOrder): Sent[] {
  const sent: Sent[] = []
  const handle = async ({ request }: { request: Request }) => {
    const body = await request.json()
    sent.push({ url: new URL(request.url).pathname.replace(`/api/v1/admin/purchasing/supplier-orders/${supplierOrderId}`, ''), body })
    const result = answer(sent.at(-1)?.url ?? '', body)
    return result instanceof Response ? result : HttpResponse.json(result)
  }
  server.use(http.put(`${path}/promised-date`, handle), http.put(`${path}/paid-prices`, handle), http.post(`${path}/in-transit`, handle), http.post(`${path}/undo`, handle))
  return sent
}

beforeEach(() => {
  vi.spyOn(window, 'open').mockReturnValue(null)
})

describe('"Receber itens" on the pedido de compra', () => {
  const receiver: Permission[] = [...buyer, 'receiving:write']

  it('opens Receber itens narrowed to this pedido de compra, for a role that receives', async () => {
    const { wrapper } = await openOrder(supplierOrderAnswer(), 'admin', receiver)

    expect(link(wrapper, 'Receber itens').attributes('href')).toBe(`/compras/receber?pedido=${supplierOrderId}`)
  })

  it('is not offered without receiving:write', async () => {
    const { wrapper } = await openOrder()

    expect(wrapper.findAll('a').map((a) => a.text())).not.toContain('Receber itens')
  })

  it('is not offered once nothing is left to receive', async () => {
    const order = supplierOrderAnswer()
    const { wrapper } = await openOrder({ ...order, lines: order.lines.map((line) => ({ ...line, status: 'received' as const })) }, 'admin', receiver)

    expect(wrapper.findAll('a').map((a) => a.text())).not.toContain('Receber itens')
  })

  it('is not offered on an undone pedido de compra', async () => {
    const order = supplierOrderAnswer()
    const { wrapper } = await openOrder({ ...order, state: 'undone', canUndo: false }, 'admin', receiver)

    expect(wrapper.findAll('a').map((a) => a.text())).not.toContain('Receber itens')
  })
})

describe('the pedido de compra screen', () => {
  it('shows the C-number, the supplier, who sent it and when, and the promised date', async () => {
    const { wrapper } = await openOrder()

    expect(wrapper.get('h1').text()).toBe('Pedido de compra C-0012')
    expect(wrapper.text()).toContain('Couros Paulista')
    expect(wrapper.text()).toContain('Enviado em 27/11/2026 por Ana Souza')
    expect(wrapper.text()).toContain('Prazo do fornecedor: Sem prazo')
    expect(wrapper.get('h1').element.parentElement?.textContent).toContain('Enviado')
  })

  it('opens the supplier chat with exactly the link the API sent', async () => {
    const { wrapper } = await openOrder()

    await button(wrapper, 'Abrir conversa no WhatsApp').trigger('click')

    expect(window.open).toHaveBeenCalledWith(links.chat, '_blank', 'noopener')
  })

  it('lists the lines with the words of the design document', async () => {
    const { wrapper } = await openOrder()

    expect(wrapper.findAll('thead th').map((th) => th.text())).toEqual(['Item', 'Qtd', 'Pedido', 'Precisa chegar até', 'Previsto', 'Situação'])
    const rows = wrapper.findAll('tbody tr').map((row) => plain(row.text()))
    expect(rows[0]).toContain('Banco de couro CB 500X')
    expect(rows[0]).toContain('Cor: Preto')
    expect(rows[0]).toContain('P-0231')
    expect(rows[0]).toContain('09/12/2026')
    expect(rows[0]).toContain('08/12/2026')
    expect(rows[0]).toContain('Enviada ao fornecedor')
    expect(rows[1]).toContain('P-0232')
    expect(link(wrapper, 'P-0232').attributes('href')).toBe(`/pedidos/${orderIds.second}`)
  })

  it('names every status of a line, and "Avisar o fornecedor" for a notice', async () => {
    const order = supplierOrderAnswer(false, {
      lines: [
        purchaseLine(1, { status: 'in_transit' }),
        purchaseLine(2, { status: 'received' }),
        purchaseLine(3, { status: 'defective', checkNote: 'Costura solta' }),
        purchaseLine(4, { status: 'cancelled' }),
        purchaseLine(5, { notice: 'order_cancelled' }),
      ],
      canUndo: false,
    })
    const { wrapper } = await openOrder(order)

    const situation = wrapper.findAll('tbody tr').map((row) => row.get('td[data-label="Situação"]').text())
    expect(situation).toEqual(['Em trânsito', 'Recebida', 'Com defeito (reposição em Compras)', 'Cancelada', 'Avisar o fornecedor'])
    const tones = wrapper.findAll('tbody tr').slice(0, 4).map((row) => row.get('[data-tone]').attributes('data-tone'))
    expect(tones).toEqual(['waiting', 'ready', 'waiting', 'neutral'])
    expect(wrapper.text()).toContain('Costura solta')
  })

  it('shows "Pode atrasar" and "Atrasada" on the lines the API alerted', async () => {
    const order = supplierOrderAnswer(false, { lines: [purchaseLine(1, { alerts: ['at_risk'] }), purchaseLine(2, { alerts: ['at_risk', 'late'] }), purchaseLine(3)] })
    const { wrapper } = await openOrder(order)

    const previsto = wrapper.findAll('tbody tr').map((row) => row.get('td[data-label="Previsto"]').text())
    expect(previsto[0]).toContain('Pode atrasar')
    expect(previsto[1]).toContain('Atrasada')
    expect(previsto[2]).not.toMatch(/atrasa/i)
  })

  it('says when the pedido de compra is not there', async () => {
    signedInAs('admin')
    server.use(http.get(path, () => problem(404, 'not_found', 'x')), ...serve().handlers)

    const { wrapper } = await openApp(`/compras/pedidos/${supplierOrderId}`)
    await vi.waitFor(() => expect(wrapper.get('[role=alert]').text()).toBe('Pedido de compra não encontrado.'))

    expect(link(wrapper, 'Voltar para Compras').attributes('href')).toBe('/compras?tab=enviadas')
  })

  it('shows cost and paid price to the owner, and neither to an admin', async () => {
    const owner = supplierOrderAnswer(true, { lines: [purchaseLine(1, { cost: brl(45000), paidCost: brl(44000) }), purchaseLine(2, { cost: brl(1000), paidCost: null })] })
    const { wrapper: ownerView } = await openOrder(owner, 'owner')
    expect(ownerView.findAll('thead th').map((th) => th.text())).toEqual(expect.arrayContaining(['Custo (por unidade)', 'Preço pago (por unidade)']))
    const cells = ownerView.findAll('tbody tr').map((row) => plain(row.text()))
    expect(cells[0]).toContain('R$ 450,00')
    expect(cells[0]).toContain('R$ 440,00')
    expect(button(ownerView, 'Informar preço pago')).toBeTruthy()

    const { wrapper: adminView } = await openOrder(supplierOrderAnswer(false), 'admin')
    expect(adminView.text()).not.toContain('Custo')
    expect(adminView.text()).not.toContain('Preço pago')
    expect(adminView.findAll('button').map((b) => b.text())).not.toContain('Informar preço pago')
  })
})

describe('"Informar prazo"', () => {
  const redated = supplierOrderAnswer(false, {
    promisedDate: '2026-12-03',
    version: 2,
    lines: [purchaseLine(1, { promisedDate: '2026-12-03', expectedDate: '2026-12-10' }), purchaseLine(2, { promisedDate: '2026-12-05', expectedDate: '2026-12-12', name: 'Capa de banco', alerts: ['at_risk'], orderNumber: 'P-0232' })],
  })

  it('saves the date for every item and a different one for a line, and the lines show the new dates', async () => {
    const { wrapper } = await openOrder()
    const sent = answerWrites(() => redated)

    await button(wrapper, 'Informar prazo').trigger('click')
    const dialog = wrapper.get('[role=dialog]')
    expect(dialog.get('h2').text()).toBe('Informar prazo')
    await field(dialog, 'Data prometida pelo fornecedor').setValue('2026-12-03')
    await field(dialog, '1× Capa de banco (P-0232)').setValue('2026-12-05')
    await button(dialog, 'Salvar prazo').trigger('click')
    await flushPromises()

    expect(sent).toEqual([{ url: '/promised-date', body: { version: 1, date: '2026-12-03', lines: [{ lineId: purchaseLine(2).id, date: '2026-12-05' }] } }])
    expect(wrapper.find('[role=dialog]').exists()).toBe(false)
    expect(wrapper.text()).toContain('Prazo salvo. As datas previstas dos pedidos foram atualizadas.')
    expect(wrapper.text()).toContain('Prazo do fornecedor: 03/12/2026')
    const previsto = wrapper.findAll('tbody tr').map((row) => row.get('td[data-label="Previsto"]').text())
    expect(previsto[0]).toContain('10/12/2026')
    expect(previsto[1]).toContain('12/12/2026')
  })

  it('names the item the new dates left late, with a link to its order', async () => {
    const { wrapper } = await openOrder()
    answerWrites(() => redated)

    await button(wrapper, 'Informar prazo').trigger('click')
    await field(wrapper.get('[role=dialog]'), 'Data prometida pelo fornecedor').setValue('2026-12-03')
    await button(wrapper.get('[role=dialog]'), 'Salvar prazo').trigger('click')
    await flushPromises()

    const warning = wrapper.findAll('[role=alert]').find((alert) => alert.text().includes('pode atrasar a entrega'))
    expect(warning?.text()).toBe('Capa de banco do pedido P-0232 pode atrasar a entrega. Veja o pedido para resolver.')
    expect(warning?.get('a').attributes('href')).toBe(`/pedidos/${purchaseLine(2).orderId}`)
  })

  it('sends no per-line dates when none differs', async () => {
    const { wrapper } = await openOrder()
    const sent = answerWrites(() => redated)

    await button(wrapper, 'Informar prazo').trigger('click')
    await field(wrapper.get('[role=dialog]'), 'Data prometida pelo fornecedor').setValue('2026-12-03')
    await button(wrapper.get('[role=dialog]'), 'Salvar prazo').trigger('click')
    await flushPromises()

    expect(sent[0]?.body).toEqual({ version: 1, date: '2026-12-03' })
  })

  it('asks for a date before calling the API, and shows the API refusal of a past date in the dialog', async () => {
    const { wrapper } = await openOrder()
    const sent = answerWrites(() => problem(400, 'invalid', 'x'))
    await button(wrapper, 'Informar prazo').trigger('click')
    const dialog = wrapper.get('[role=dialog]')

    await dialog.get('form').trigger('submit')
    await flushPromises()
    expect(dialog.text()).toContain('Escolha uma data a partir de hoje.')
    expect(sent).toHaveLength(0)

    await field(dialog, 'Data prometida pelo fornecedor').setValue('2020-01-01')
    await dialog.get('form').trigger('submit')
    await flushPromises()
    expect(sent).toHaveLength(1)
    expect(wrapper.get('[role=dialog] [role=alert]').text()).toBe('Escolha uma data a partir de hoje.')
  })

  it('puts the conflict on the page with "Recarregar", which reads the pedido again', async () => {
    const { wrapper } = await openOrder()
    answerWrites(() => problem(409, 'version_conflict', 'x'))
    await button(wrapper, 'Informar prazo').trigger('click')
    await field(wrapper.get('[role=dialog]'), 'Data prometida pelo fornecedor').setValue('2026-12-03')
    await button(wrapper.get('[role=dialog]'), 'Salvar prazo').trigger('click')
    await flushPromises()

    expect(wrapper.get('[role=alert]').text()).toContain('Outra pessoa alterou este pedido de compra. Recarregue para ver a versão atual.')
    server.use(http.get(path, () => HttpResponse.json({ ...supplierOrderAnswer(), version: 5, promisedDate: '2026-12-01' })))
    await button(wrapper, 'Recarregar').trigger('click')
    await vi.waitFor(() => expect(wrapper.text()).toContain('Prazo do fornecedor: 01/12/2026'))
    expect(wrapper.text()).not.toContain('Outra pessoa alterou')
  })
})

describe('a version conflict from a dialog', () => {
  async function conflictFrom(open: string, confirm: string, order: SupplierOrder, role: 'owner' | 'admin') {
    const { wrapper } = await openOrder(order, role)
    answerWrites(() => problem(409, 'version_conflict', 'x'))
    await button(wrapper, open).trigger('click')
    const dialog = wrapper.get('[role=dialog]')
    if (open === 'Informar prazo') await field(dialog, 'Data prometida pelo fornecedor').setValue('2026-12-03')
    await button(dialog, confirm).trigger('click')
    await flushPromises()
    return wrapper
  }

  it.each([
    ['Informar prazo', 'Salvar prazo', 'admin'],
    ['Informar preço pago', 'Salvar preços', 'owner'],
    ['Desfazer envio', 'Desfazer envio', 'admin'],
  ] as const)('closes %s and shows the message and "Recarregar" on the page', async (open, confirm, role) => {
    const wrapper = await conflictFrom(open, confirm, supplierOrderAnswer(role === 'owner'), role)

    expect(wrapper.find('[role=dialog]').exists()).toBe(false)
    expect(wrapper.get('[role=alert]').text()).toContain('Outra pessoa alterou este pedido de compra. Recarregue para ver a versão atual.')
    expect(button(wrapper, 'Recarregar')).toBeTruthy()
  })
})

describe('"Marcar como em trânsito"', () => {
  it('marks the sent lines and says so, and the button leaves', async () => {
    const { wrapper } = await openOrder()
    const sent = answerWrites(() => supplierOrderAnswer(false, { state: 'in_transit', version: 2, lines: [purchaseLine(1, { status: 'in_transit' }), purchaseLine(2, { status: 'in_transit' })] }))

    await button(wrapper, 'Marcar como em trânsito').trigger('click')
    await flushPromises()

    expect(sent).toEqual([{ url: '/in-transit', body: { version: 1 } }])
    expect(wrapper.text()).toContain('Itens em trânsito.')
    expect(wrapper.findAll('button').map((b) => b.text())).not.toContain('Marcar como em trânsito')
  })
})

describe('"Desfazer envio"', () => {
  const undone = supplierOrderAnswer(false, { state: 'undone', version: 2, total: 0, canUndo: false, lines: [purchaseLine(1, { status: 'cancelled' }), purchaseLine(2, { status: 'cancelled' })] })

  it('asks first, with the words of the design document, then undoes it', async () => {
    const { wrapper } = await openOrder()
    const sent = answerWrites(() => undone)

    await button(wrapper, 'Desfazer envio').trigger('click')
    const dialog = wrapper.get('[role=dialog]')
    expect(dialog.text()).toContain('Desfazer o envio do pedido de compra C-0012? Os itens voltam para A comprar. O número C-0012 não será usado de novo.')
    expect(sent).toHaveLength(0)

    await button(dialog, 'Desfazer envio').trigger('click')
    await flushPromises()

    expect(sent).toEqual([{ url: '/undo', body: { version: 1 } }])
    expect(wrapper.text()).toContain('Envio desfeito.')
    expect(wrapper.text()).toContain('Desfeito')
    expect(wrapper.findAll('button').map((b) => b.text())).not.toEqual(expect.arrayContaining(['Desfazer envio']))
  })

  it('"Voltar" closes the question and changes nothing', async () => {
    const { wrapper } = await openOrder()
    const sent = answerWrites(() => undone)
    await button(wrapper, 'Desfazer envio').trigger('click')

    await button(wrapper.get('[role=dialog]'), 'Voltar').trigger('click')

    expect(wrapper.find('[role=dialog]').exists()).toBe(false)
    expect(sent).toHaveLength(0)
  })

  it('shows the refusal when something was received meanwhile', async () => {
    const { wrapper } = await openOrder()
    answerWrites(() => problem(409, 'supplier_order_received', 'x'))
    await button(wrapper, 'Desfazer envio').trigger('click')

    await button(wrapper.get('[role=dialog]'), 'Desfazer envio').trigger('click')
    await flushPromises()

    expect(wrapper.get('[role=dialog] [role=alert]').text()).toBe('Este pedido de compra já tem itens recebidos e não pode ser desfeito.')
  })

  it('offers no undo once something was received, and says why', async () => {
    const { wrapper } = await openOrder(supplierOrderAnswer(false, { canUndo: false, received: 1, lines: [purchaseLine(1, { status: 'received' }), purchaseLine(2)] }))

    expect(wrapper.findAll('button').map((b) => b.text())).not.toContain('Desfazer envio')
    expect(wrapper.text()).toContain('Este pedido de compra já tem itens recebidos e não pode ser desfeito.')
  })
})

describe('"Informar preço pago"', () => {
  const owner = supplierOrderAnswer(true, { lines: [purchaseLine(1, { cost: brl(45000), paidCost: null }), purchaseLine(2, { cost: brl(1000), paidCost: brl(900) })] })
  const priced = supplierOrderAnswer(true, { version: 2, lines: [purchaseLine(1, { cost: brl(45000), paidCost: brl(8990) }), purchaseLine(2, { cost: brl(1000), paidCost: brl(900) })] })

  it('starts each field from the paid price or the cost, and sends integer cents', async () => {
    const { wrapper } = await openOrder(owner, 'owner')
    const sent = answerWrites(() => priced)

    await button(wrapper, 'Informar preço pago').trigger('click')
    const dialog = wrapper.get('[role=dialog]')
    const inputs = dialog.findAll('input')
    expect(inputs.map((input) => (input.element as HTMLInputElement).value)).toEqual(['450,00', '9,00'])
    expect(dialog.findAll('label').map((label) => label.text())).toEqual(['Preço pago (por unidade)', 'Preço pago (por unidade)'])

    await inputs[0]?.setValue('89,90')
    await button(dialog, 'Salvar preços').trigger('click')
    await flushPromises()

    expect(sent).toEqual([{ url: '/paid-prices', body: { version: 1, prices: [{ lineId: purchaseLine(1).id, cents: 8990 }, { lineId: purchaseLine(2).id, cents: 900 }] } }])
    expect(wrapper.text()).toContain('Preços salvos. As margens do Financeiro usam o preço pago.')
    expect(plain(wrapper.findAll('tbody tr')[0]?.text() ?? '')).toContain('R$ 89,90')
  })

  it('asks to check a value that is not an amount, without calling the API', async () => {
    const { wrapper } = await openOrder(owner, 'owner')
    const sent = answerWrites(() => priced)
    await button(wrapper, 'Informar preço pago').trigger('click')
    const dialog = wrapper.get('[role=dialog]')

    await dialog.findAll('input')[0]?.setValue('abc')
    await button(dialog, 'Salvar preços').trigger('click')
    await flushPromises()

    expect(dialog.text()).toContain('Confira o valor.')
    expect(sent).toHaveLength(0)
  })

  it('shows the refusal of a line that closed meanwhile', async () => {
    const { wrapper } = await openOrder(owner, 'owner')
    answerWrites(() => problem(409, 'line_closed', 'x'))
    await button(wrapper, 'Informar preço pago').trigger('click')

    await button(wrapper.get('[role=dialog]'), 'Salvar preços').trigger('click')
    await flushPromises()

    expect(wrapper.get('[role=dialog] [role=alert]').text()).toBe('Esta linha já foi recebida ou cancelada. Recarregue para ver a situação atual.')
  })
})

describe('the phone layout', () => {
  // The layout itself is CSS (the Playwright phone project checks 390 px); these are the structures it relies on.
  it('turns the lines into cards with a labelled cell for every column', async () => {
    const { wrapper } = await openOrder()

    expect(wrapper.get('table').classes()).toContain('enc-cards')
    expect(wrapper.findAll('tbody tr')[0]?.findAll('td').map((cell) => cell.attributes('data-label'))).toEqual(['Qtd', 'Pedido', 'Precisa chegar até', 'Previsto', 'Situação'])
  })

  it('puts the buttons of every sheet in the sticky footer, a direct child of the form', async () => {
    const { wrapper } = await openOrder(supplierOrderAnswer(true), 'owner')

    await button(wrapper, 'Informar prazo').trigger('click')
    expect(wrapper.get('[role=dialog] form > .enc-sheet-actions').findAll('button').map((b) => b.text())).toEqual(['Salvar prazo', 'Cancelar'])
    await button(wrapper.get('[role=dialog]'), 'Cancelar').trigger('click')

    await button(wrapper, 'Informar preço pago').trigger('click')
    expect(wrapper.get('[role=dialog] form > .enc-sheet-actions').findAll('button').map((b) => b.text())).toEqual(['Salvar preços', 'Cancelar'])
    await button(wrapper.get('[role=dialog]'), 'Cancelar').trigger('click')

    await button(wrapper, 'Desfazer envio').trigger('click')
    expect(wrapper.get('[role=dialog] .enc-sheet-actions').findAll('button').map((b) => b.text())).toEqual(['Desfazer envio', 'Voltar'])
  })
})
