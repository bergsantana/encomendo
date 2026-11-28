import type { Permission, Supplier, SupplierOrder } from '@encomendo/api'
import { enableAutoUnmount, flushPromises, type VueWrapper } from '@vue/test-utils'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'

import { button, closeApps, field, link, openApp } from '../../testing/app'
import { authSession, problem } from '../../testing/fixtures'
import { encomendaMontagem, settingsUsing } from '../../testing/orders'
import { todayHandler } from '../../testing/today'
import {
  type Backend,
  couros,
  links,
  motoPecas,
  noticeFor,
  serve,
  summaryOf,
  supplierOrderAnswer,
  supplierOrderId,
  toBuyAnswer,
  toBuyItem,
} from './testkit'

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

const signedInAs = (role: 'owner' | 'admin' | 'staff' | 'viewer', permissions: Permission[]) =>
  server.use(http.post('*/auth/refresh', () => HttpResponse.json(authSession({ role, permissions }))))

/** use answers the reads of Compras; `extra` handlers go first, so a test can change one answer. */
function use(backend: Backend = {}, ...extra: Parameters<typeof server.use>) {
  const { handlers, seen } = serve(backend)
  server.use(...extra, ...handlers)
  return seen
}

const plain = (text: string) => text.replace(/\u00a0/g, ' ')

async function openCompras(path = '/compras', waitFor = 'Couros Paulista') {
  const app = await openApp(path)
  await vi.waitFor(() => expect(app.wrapper.text()).toContain(waitFor))
  return app
}

/** card finds a supplier's card by its name. */
function card(wrapper: VueWrapper, name: string) {
  const found = wrapper.findAll('article').find((article) => article.get('h2').text() === name)
  if (found === undefined) throw new Error(`no card for ${name}`)
  return found
}

beforeEach(() => {
  signedInAs('admin', buyer)
  vi.spyOn(window, 'open').mockReturnValue(null)
  use()
})

describe('the Compras menu entry and route', () => {
  it('sits after Pedidos and before Catálogo, for a role with purchase:write', async () => {
    const { wrapper } = await openApp('/hoje')
    server.use(todayHandler())

    const entries = wrapper.findAll('nav a.menu-link').map((a) => a.text())
    expect(entries.slice(0, 4)).toEqual(['Hoje', 'Pedidos', 'Compras', 'Catálogo'])
    expect(link(wrapper, 'Compras').attributes('href')).toBe('/compras')
  })

  it('is hidden from a role without purchase:write, and its address leads to Hoje', async () => {
    signedInAs('staff', ['order:read', 'receiving:write'])

    const { wrapper, router } = await openApp('/compras')

    expect(router.currentRoute.value.name).toBe('hoje')
    expect(wrapper.findAll('a').map((a) => a.text())).not.toContain('Compras')
  })

  it('keeps "Compras" marked on a pedido de compra', async () => {
    const { wrapper } = await openApp(`/compras/pedidos/${supplierOrderId}`)
    await vi.waitFor(() => expect(wrapper.text()).toContain('C-0012'))

    expect(link(wrapper, 'Compras').classes()).toContain('menu-link--active')
  })
})

describe('the tabs', () => {
  it('names the four tabs and opens "A comprar" first', async () => {
    const { wrapper } = await openCompras()

    expect(wrapper.findAll('[role=tab]').map((tab) => tab.text())).toEqual(['A comprar', 'Enviadas', 'Receber itens', 'Fornecedores'])
    expect(wrapper.get('[role=tab][aria-selected=true]').text()).toBe('A comprar')
  })

  it('keeps the tab in ?tab= and opens on it after a reload', async () => {
    const { wrapper, router } = await openCompras()

    await wrapper.get('#tab-enviadas').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.query.tab).toBe('enviadas')

    const reloaded = await openCompras('/compras?tab=fornecedores', 'Buscar fornecedor')
    expect(reloaded.wrapper.get('[role=tab][aria-selected=true]').text()).toBe('Fornecedores')
  })

  it('falls back to "A comprar" for a tab it does not know', async () => {
    const { wrapper } = await openCompras('/compras?tab=outra')

    expect(wrapper.get('[role=tab][aria-selected=true]').text()).toBe('A comprar')
  })
})

describe('A comprar', () => {
  it('shows a card per supplier, with its WhatsApp and the count of entries', async () => {
    const { wrapper } = await openCompras()

    const first = card(wrapper, 'Couros Paulista')
    expect(first.text()).toContain('(11) 98765-4321')
    expect(first.text()).toContain('2 itens')
    expect(card(wrapper, 'Moto Peças').text()).toContain('1 item')
    expect(card(wrapper, 'Moto Peças').text()).not.toContain('1 itens')
  })

  it('lists each line with its choices, order, limit date and the Reposição badge, all boxes checked', async () => {
    const { wrapper } = await openCompras()

    const first = card(wrapper, 'Couros Paulista')
    const lines = first.findAll('li')
    expect(lines).toHaveLength(2)
    expect(plain(lines[0]?.text() ?? '')).toContain('2× Banco de couro CB 500X')
    expect(lines[0]?.text()).toContain('Cor: Preto')
    expect(lines[0]?.text()).toContain('Pedido P-0231')
    expect(lines[0]?.text()).toContain('Precisa chegar até 09/12/2026')
    expect(lines[0]?.text()).not.toContain('Reposição')
    expect(lines[1]?.text()).toContain('1× Capa de banco')
    expect(lines[1]?.text()).toContain('Reposição')
    for (const box of first.findAll('input[type=checkbox]')) expect((box.element as HTMLInputElement).checked).toBe(true)
  })

  it('opens the order from "Pedido P-0231"', async () => {
    const { wrapper } = await openCompras()

    const order = card(wrapper, 'Couros Paulista').findAll('a').find((a) => a.text() === 'P-0231')
    expect(order?.attributes('href')).toBe(`/pedidos/${toBuyItem(1).orderId}`)
  })

  it('shows "Custo (por unidade)" to the owner and nothing to an admin', async () => {
    signedInAs('owner', buyer)
    use({ toBuy: toBuyAnswer(true) })
    const owner = await openCompras()
    expect(plain(card(owner.wrapper, 'Couros Paulista').text())).toContain('Custo (por unidade) R$ 450,00')

    signedInAs('admin', buyer)
    use({ toBuy: toBuyAnswer(false) })
    const admin = await openCompras()
    expect(admin.wrapper.text()).not.toContain('Custo')
  })

  it('says the list may not be complete when the API read only part of the orders', async () => {
    use({ toBuy: { ...toBuyAnswer(), truncated: true } })

    const { wrapper } = await openCompras()

    expect(wrapper.get('[role=status]').text()).toBe('Mostrando só os itens dos 500 pedidos ativos com entrega mais próxima. A lista pode não ter tudo.')
  })

  it('says nothing about it when the list is whole', async () => {
    const { wrapper } = await openCompras()

    expect(wrapper.text()).not.toContain('Mostrando só')
  })

  it('tells "Nada para comprar agora." with the hint for deposit-first stores', async () => {
    use({ toBuy: { suppliers: [], truncated: false } })

    const { wrapper } = await openCompras('/compras', 'Nada para comprar agora.')

    expect(wrapper.text()).toContain('Os itens aparecem aqui quando o cliente paga o sinal.')
  })

  it('words the empty hint for "Liberar compras antes do sinal"', async () => {
    use({ toBuy: { suppliers: [], truncated: false }, settings: settingsUsing(encomendaMontagem, { purchaseBeforeDeposit: true }) })

    const { wrapper } = await openCompras('/compras', 'Nada para comprar agora.')
    await vi.waitFor(() => expect(wrapper.text()).toContain('quando o pedido sai do rascunho.'))
  })

  it('says it is loading while the first read is on its way', async () => {
    let answer = () => {}
    const waiting = new Promise<void>((resolve) => (answer = resolve))
    use({}, http.get('*/api/v1/admin/purchasing/to-buy', async () => {
      await waiting
      return HttpResponse.json(toBuyAnswer())
    }))

    const { wrapper } = await openApp('/compras')
    expect(wrapper.text()).toContain('Carregando as compras…')
    answer()
    await vi.waitFor(() => expect(wrapper.text()).toContain('Couros Paulista'))
  })

  it('offers "Tentar de novo" when the read fails', async () => {
    use({}, http.get('*/api/v1/admin/purchasing/to-buy', () => new HttpResponse(null, { status: 500 })))
    const { wrapper } = await openApp('/compras')
    await vi.waitFor(() => expect(wrapper.text()).toContain('O servidor não respondeu'))

    use({})
    await button(wrapper, 'Tentar de novo').trigger('click')
    await vi.waitFor(() => expect(wrapper.text()).toContain('Couros Paulista'))
  })
})

describe('"Ver mensagem"', () => {
  it("shows the API's text for the checked lines, and asks again when a box changes", async () => {
    const seen = use()
    const { wrapper } = await openCompras()
    const first = card(wrapper, 'Couros Paulista')

    await button(first, 'Ver mensagem').trigger('click')
    await vi.waitFor(() => expect(first.text()).toContain('Pedido de compra C-0013 para 2 item(ns)'))
    expect(seen.previews[0]).toEqual({ supplierId: couros.id, orderItemIds: [toBuyItem(1).orderItemId, toBuyItem(2).orderItemId] })

    await field(first, '2× Banco de couro CB 500X').setValue(false)
    await vi.waitFor(() => expect(first.text()).toContain('Pedido de compra C-0013 para 1 item(ns)'))
    expect(seen.previews[1]).toEqual({ supplierId: couros.id, orderItemIds: [toBuyItem(2).orderItemId] })
  })

  it('copies the text with "Copiar mensagem"', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
    const { wrapper } = await openCompras()
    const first = card(wrapper, 'Couros Paulista')
    await button(first, 'Ver mensagem').trigger('click')
    await vi.waitFor(() => expect(first.text()).toContain('Copiar mensagem'))

    await button(first, 'Copiar mensagem').trigger('click')
    await flushPromises()

    expect(writeText).toHaveBeenCalledWith('Pedido de compra C-0013 para 2 item(ns)')
    expect(wrapper.text()).toContain('Mensagem copiada.')
  })

  it('says so when the browser refuses to copy', async () => {
    Object.defineProperty(navigator, 'clipboard', { value: undefined, configurable: true })
    const { wrapper } = await openCompras()
    const first = card(wrapper, 'Couros Paulista')
    await button(first, 'Ver mensagem').trigger('click')
    await vi.waitFor(() => expect(first.text()).toContain('Copiar mensagem'))

    await button(first, 'Copiar mensagem').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('Não foi possível copiar a mensagem.')
  })

  it('closes the open message when every box is taken off, instead of loading forever', async () => {
    const { wrapper } = await openCompras()
    const second = card(wrapper, 'Moto Peças')
    await button(second, 'Ver mensagem').trigger('click')
    await vi.waitFor(() => expect(second.find('pre').exists()).toBe(true))

    await field(second, '4× Pastilha de freio').setValue(false)
    await flushPromises()

    expect(second.find('pre').exists()).toBe(false)
    expect(second.text()).not.toContain('Carregando a mensagem')
    expect(second.text()).not.toContain('Copiar mensagem')
  })

  it('needs at least one box: with none, both buttons wait and the hint says why', async () => {
    const { wrapper } = await openCompras()
    const second = card(wrapper, 'Moto Peças')
    expect(second.text()).toContain('Os itens marcados ficam como Enviada ao fornecedor.')

    await field(second, '4× Pastilha de freio').setValue(false)

    expect(second.text()).toContain('Marque ao menos um item.')
    expect(second.text()).not.toContain('Carregando a mensagem')
    expect(button(second, 'Enviar pelo WhatsApp').attributes('disabled')).toBeDefined()
    expect(button(second, 'Ver mensagem').attributes('disabled')).toBeDefined()
  })
})

describe('"Enviar pelo WhatsApp"', () => {
  const sentAnswer = (order: SupplierOrder = supplierOrderAnswer()) => ({ supplierOrder: order, whatsappUrl: links.send, text: 'Texto do pedido' })

  /** captureSend answers the send and keeps what it got. */
  function captureSend(answer: () => Response | Promise<Response>) {
    const calls: { key: string | null; body: { supplierId: string; orderItemIds: string[] } }[] = []
    server.use(
      http.post('*/api/v1/admin/purchasing/supplier-orders', async ({ request }) => {
        calls.push({ key: request.headers.get('Idempotency-Key'), body: (await request.json()) as never })
        return answer()
      }),
    )
    return calls
  }

  it('sends the checked items with a key, opens exactly the API link and keeps the follow-up buttons', async () => {
    const calls = captureSend(() => HttpResponse.json(sentAnswer(), { status: 201 }))
    const { wrapper } = await openCompras()
    await field(card(wrapper, 'Couros Paulista'), '1× Capa de banco').setValue(false)

    await button(card(wrapper, 'Couros Paulista'), 'Enviar pelo WhatsApp').trigger('click')
    await vi.waitFor(() => expect(wrapper.text()).toContain('Pedido de compra C-0012 criado.'))

    expect(calls).toHaveLength(1)
    expect(calls[0]?.key).toMatch(/^[0-9a-f-]{36}$/)
    expect(calls[0]?.body).toEqual({ supplierId: couros.id, orderItemIds: [toBuyItem(1).orderItemId] })
    expect(window.open).toHaveBeenCalledOnce()
    expect(window.open).toHaveBeenCalledWith(links.send, '_blank', 'noopener')
    expect(wrapper.findAll('button').map((b) => b.text())).toEqual(expect.arrayContaining(['Abrir o WhatsApp', 'Copiar mensagem']))
    expect(link(wrapper, 'Ver pedido de compra').attributes('href')).toBe(`/compras/pedidos/${supplierOrderId}`)

    vi.mocked(window.open).mockClear()
    await button(wrapper, 'Abrir o WhatsApp').trigger('click')
    expect(window.open).toHaveBeenCalledWith(links.send, '_blank', 'noopener')
  })

  it('copies the text of the send', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
    captureSend(() => HttpResponse.json(sentAnswer(), { status: 201 }))
    const { wrapper } = await openCompras()
    await button(card(wrapper, 'Couros Paulista'), 'Enviar pelo WhatsApp').trigger('click')
    await vi.waitFor(() => expect(wrapper.text()).toContain('Pedido de compra C-0012 criado.'))

    await button(wrapper, 'Copiar mensagem').trigger('click')
    await flushPromises()

    expect(writeText).toHaveBeenCalledWith('Texto do pedido')
  })

  it('reads the list again after it, so the sent items leave "A comprar"', async () => {
    captureSend(() => HttpResponse.json(sentAnswer(), { status: 201 }))
    const { wrapper } = await openCompras()
    server.use(http.get('*/api/v1/admin/purchasing/to-buy', () => HttpResponse.json({ suppliers: [toBuyAnswer().suppliers[1]], truncated: false })))

    await button(card(wrapper, 'Couros Paulista'), 'Enviar pelo WhatsApp').trigger('click')

    await vi.waitFor(() => expect(wrapper.findAll('article').map((a) => a.get('h2').text())).toEqual(['Moto Peças']))
    expect(wrapper.text()).toContain('Pedido de compra C-0012 criado.')
  })

  it('retries with the same key after a lost answer, and a new key for a new pick', async () => {
    let attempt = 0
    const calls = captureSend(() => {
      attempt += 1
      return attempt === 1 ? HttpResponse.error() : HttpResponse.json(sentAnswer(), { status: 201 })
    })
    const { wrapper } = await openCompras()
    const first = card(wrapper, 'Couros Paulista')

    await button(first, 'Enviar pelo WhatsApp').trigger('click')
    await vi.waitFor(() => expect(wrapper.text()).toContain('Sem conexão com o servidor'))
    await button(first, 'Enviar pelo WhatsApp').trigger('click')
    await vi.waitFor(() => expect(wrapper.text()).toContain('Pedido de compra C-0012 criado.'))

    expect(calls).toHaveLength(2)
    expect(calls[1]?.key).toBe(calls[0]?.key)
    expect(window.open).toHaveBeenCalledOnce()
  })

  it('refuses an item that is no longer in "A comprar" and reads the list again', async () => {
    captureSend(() => problem(409, 'item_not_to_buy', 'x'))
    const { wrapper } = await openCompras()
    server.use(http.get('*/api/v1/admin/purchasing/to-buy', () => HttpResponse.json({ suppliers: [toBuyAnswer().suppliers[1]], truncated: false })))

    await button(card(wrapper, 'Couros Paulista'), 'Enviar pelo WhatsApp').trigger('click')

    await vi.waitFor(() => expect(wrapper.get('[role=alert]').text()).toBe('Algum item marcado já não está em A comprar. A lista foi atualizada; confira e envie de novo.'))
    await vi.waitFor(() => expect(wrapper.findAll('article')).toHaveLength(1))
    expect(window.open).not.toHaveBeenCalled()
  })

  it('refuses an order that was handed over or cancelled meanwhile', async () => {
    captureSend(() => problem(409, 'order_closed', 'x'))
    const { wrapper } = await openCompras()

    await button(card(wrapper, 'Moto Peças'), 'Enviar pelo WhatsApp').trigger('click')

    await vi.waitFor(() => expect(card(wrapper, 'Moto Peças').get('[role=alert]').text()).toContain('foi entregue ou cancelado'))
    expect(card(wrapper, 'Couros Paulista').find('[role=alert]').exists()).toBe(false)
  })

  it('forgets the last send, messages and preview when Compras is shown again', async () => {
    captureSend(() => HttpResponse.json(sentAnswer(), { status: 201 }))
    use({ orders: [] })
    const { wrapper } = await openCompras()
    await button(card(wrapper, 'Moto Peças'), 'Ver mensagem').trigger('click')
    await vi.waitFor(() => expect(wrapper.find('pre').exists()).toBe(true))
    await button(card(wrapper, 'Couros Paulista'), 'Enviar pelo WhatsApp').trigger('click')
    await vi.waitFor(() => expect(wrapper.text()).toContain('Pedido de compra C-0012 criado.'))

    await wrapper.get('#tab-enviadas').trigger('click')
    await flushPromises()
    await wrapper.get('#tab-a-comprar').trigger('click')
    await flushPromises()

    expect(wrapper.text()).not.toContain('criado.')
    expect(wrapper.find('pre').exists()).toBe(false)
  })
})

describe('Enviadas', () => {
  const sent = supplierOrderAnswer()
  const received = supplierOrderAnswer(false, { id: '0190f3c4-5e1a-7b2c-8d3e-000000005002', number: 'C-0011', state: 'received', received: 2, promisedDate: '2026-12-03' })

  it('lists the pedidos de compra with the words of the design document', async () => {
    use({ orders: [summaryOf(sent), summaryOf(received)] })
    const { wrapper } = await openCompras('/compras?tab=enviadas', 'C-0012')

    expect(wrapper.findAll('thead th').map((th) => th.text())).toEqual(['Pedido de compra', 'Fornecedor', 'Enviado em', 'Prazo do fornecedor', 'Recebidos', 'Situação'])
    const rows = wrapper.findAll('tbody tr').map((row) => plain(row.text()))
    expect(rows[0]).toContain('C-0012')
    expect(rows[0]).toContain('Couros Paulista')
    expect(rows[0]).toContain('27/11/2026')
    expect(rows[0]).toContain('Sem prazo')
    expect(rows[0]).toContain('0 de 2')
    expect(rows[0]).toContain('Enviado')
    expect(rows[1]).toContain('03/12/2026')
    expect(rows[1]).toContain('2 de 2')
    expect(rows[1]).toContain('Recebido')
    expect(link(wrapper, 'C-0012').attributes('href')).toBe(`/compras/pedidos/${supplierOrderId}`)
  })

  it('asks for the open ones first and for all of them with "Todos"', async () => {
    const seen = use({ orders: [summaryOf(sent)] })
    const { wrapper } = await openCompras('/compras?tab=enviadas', 'C-0012')
    expect(seen.orderLists[0]?.searchParams.get('state')).toBe('open')
    expect((field(wrapper, 'Abertos').element as HTMLInputElement).checked).toBe(true)

    await field(wrapper, 'Todos').setValue(true)
    await flushPromises()

    expect(seen.orderLists.at(-1)?.searchParams.get('state')).toBe('all')
    expect((field(wrapper, 'Todos').element as HTMLInputElement).checked).toBe(true)
  })

  it('pages with "Carregar mais"', async () => {
    let page = 0
    use({}, http.get('*/api/v1/admin/purchasing/supplier-orders', ({ request }) => {
      page += 1
      const cursor = new URL(request.url).searchParams.get('cursor')
      return cursor === null ? HttpResponse.json({ items: [summaryOf(sent)], nextCursor: 'n1' }) : HttpResponse.json({ items: [summaryOf(received)] })
    }))
    const { wrapper } = await openCompras('/compras?tab=enviadas', 'C-0012')

    await button(wrapper, 'Carregar mais').trigger('click')
    await vi.waitFor(() => expect(wrapper.text()).toContain('C-0011'))

    expect(page).toBe(2)
    expect(wrapper.findAll('tbody tr')).toHaveLength(2)
    expect(wrapper.findAll('button').map((b) => b.text())).not.toContain('Carregar mais')
  })

  it('tells when there is none', async () => {
    const { wrapper } = await openCompras('/compras?tab=enviadas', 'Nenhum pedido de compra ainda.')

    expect(wrapper.find('table').exists()).toBe(false)
  })
})

describe('Avisar o fornecedor', () => {
  it('lists the notices above the tabs with the reason of each', async () => {
    use({
      notices: [
        noticeFor(1),
        noticeFor(2, { name: 'Capa de banco', quantity: 1, reason: 'item_removed', orderNumber: 'P-0232' }),
        noticeFor(3, { name: 'Pastilha de freio', quantity: 4, reason: 'item_changed' }),
      ],
    })
    const { wrapper } = await openCompras('/compras', '3 itens já enviados não são mais necessários.')

    const panel = wrapper.get('[aria-label="Avisar o fornecedor"]')
    expect(plain(panel.text())).toContain('C-0012 · Couros Paulista')
    expect(plain(panel.text())).toContain('2× Banco de couro CB 500X')
    expect(panel.text()).toContain('O pedido P-0231 foi cancelado.')
    expect(panel.text()).toContain('Capa de banco saiu do pedido P-0232.')
    expect(panel.text()).toContain('Pastilha de freio mudou no pedido P-0231.')
  })

  it('says it in the singular for one', async () => {
    use({ notices: [noticeFor(1)] })

    const { wrapper } = await openCompras('/compras', '1 item já enviado não é mais necessário.')

    expect(wrapper.text()).not.toContain('1 itens')
  })

  it('opens exactly the API link with "Avisar pelo WhatsApp"', async () => {
    use({ notices: [noticeFor(1)] })
    const { wrapper } = await openCompras('/compras', 'O pedido P-0231 foi cancelado.')

    await button(wrapper, 'Avisar pelo WhatsApp').trigger('click')

    expect(window.open).toHaveBeenCalledOnce()
    expect(window.open).toHaveBeenCalledWith(links.notice, '_blank', 'noopener')
  })

  it('resolves a notice: the line is cancelled, the notice leaves and the screen says so', async () => {
    let resolved = false
    const seen: string[] = []
    use(
      {},
      http.get('*/api/v1/admin/purchasing/notices', () => HttpResponse.json({ items: resolved ? [] : [noticeFor(1)] })),
      http.post('*/api/v1/admin/purchasing/notices/:lineId/resolve', ({ params }) => {
        seen.push(String(params.lineId))
        resolved = true
        return new HttpResponse(null, { status: 204 })
      }),
    )
    const { wrapper } = await openCompras('/compras', 'O pedido P-0231 foi cancelado.')

    await button(wrapper, 'Marcar como resolvido').trigger('click')
    await vi.waitFor(() => expect(wrapper.text()).toContain('Resolvido. A linha foi cancelada.'))

    expect(seen).toEqual([noticeFor(1).lineId])
    expect(wrapper.text()).not.toContain('O pedido P-0231 foi cancelado.')
    expect(wrapper.findAll('button').map((b) => b.text())).not.toContain('Marcar como resolvido')
  })

  it('says why a notice could not be resolved', async () => {
    use(
      { notices: [noticeFor(1)] },
      http.post('*/api/v1/admin/purchasing/notices/:lineId/resolve', () => problem(409, 'line_closed', 'x')),
    )
    const { wrapper } = await openCompras('/compras', 'O pedido P-0231 foi cancelado.')

    await button(wrapper, 'Marcar como resolvido').trigger('click')

    await vi.waitFor(() => expect(wrapper.get('[role=alert]').text()).toBe('Esta linha já foi recebida ou cancelada. Recarregue para ver a situação atual.'))
  })

  it('is not there when there is nothing to tell', async () => {
    const { wrapper } = await openCompras()

    expect(wrapper.find('[aria-label="Avisar o fornecedor"]').exists()).toBe(false)
  })
})

describe('Fornecedores', () => {
  const suppliers: Supplier[] = [
    { id: couros.id, name: couros.name, whatsappPhone: couros.phone, version: 3 },
    { id: motoPecas.id, name: motoPecas.name, whatsappPhone: motoPecas.phone, version: 1 },
  ]

  async function openFornecedores(backend: Backend = {}, ...extra: Parameters<typeof server.use>) {
    const seen = use({ suppliers, ...backend }, ...extra)
    const app = await openCompras('/compras?tab=fornecedores', 'Buscar fornecedor')
    await vi.waitFor(() => expect(app.wrapper.text()).toMatch(/Couros Paulista|Nenhum fornecedor/))
    return { ...app, seen }
  }

  it('lists the suppliers by name and WhatsApp', async () => {
    const { wrapper } = await openFornecedores()

    expect(wrapper.findAll('thead th').map((th) => th.text())).toEqual(['Fornecedor', 'WhatsApp'])
    expect(wrapper.findAll('tbody tr').map((row) => row.text())).toEqual(['Couros Paulista(11) 98765-4321', 'Moto Peças(92) 99123-4567'])
  })

  it('searches by name', async () => {
    const { wrapper, seen } = await openFornecedores()

    await field(wrapper, 'Buscar fornecedor').setValue('moto')
    await wrapper.get('form[role=search]').trigger('submit')
    await flushPromises()

    expect(seen.supplierLists.at(-1)?.searchParams.get('q')).toBe('moto')
  })

  it('tells when there is none', async () => {
    const { wrapper } = await openFornecedores({ suppliers: [] })

    await vi.waitFor(() => expect(wrapper.text()).toContain('Nenhum fornecedor ainda. Você também pode criar um fornecedor no cadastro do item.'))
  })

  it('opens a supplier with the items it supplies, and the cost only when the API sent it', async () => {
    const items = [
      { itemId: toBuyItem(1).catalogItemId, itemName: 'Banco de couro CB 500X', role: 'main' as const, leadDays: 5 },
      { itemId: toBuyItem(2).catalogItemId, itemName: 'Capa de banco', role: 'backup' as const, leadDays: 8 },
    ]
    const admin = await openFornecedores({ supplierItems: items })
    await button(admin.wrapper, 'Couros Paulista').trigger('click')
    await vi.waitFor(() => expect(admin.wrapper.find('[role=dialog] table').exists()).toBe(true))

    const dialog = admin.wrapper.get('[role=dialog]')
    expect(dialog.get('h2').text()).toBe('Fornecedor')
    expect(dialog.findAll('thead th').map((th) => th.text())).toEqual(['Item', 'Papel', 'Prazo (dias)'])
    expect(dialog.findAll('tbody tr').map((row) => row.text())).toEqual(['Banco de couro CB 500XPrincipal5', 'Capa de bancoReserva8'])

    const withCost = items.map((item) => ({ ...item, cost: { cents: 45000, currency: 'BRL' } }))
    const owner = await openFornecedores({ supplierItems: withCost })
    await button(owner.wrapper, 'Couros Paulista').trigger('click')
    await vi.waitFor(() => expect(owner.wrapper.find('[role=dialog] table').exists()).toBe(true))
    expect(owner.wrapper.findAll('[role=dialog] thead th').map((th) => th.text())).toEqual(['Item', 'Papel', 'Prazo (dias)', 'Custo'])
    expect(plain(owner.wrapper.get('[role=dialog] tbody tr').text())).toContain('R$ 450,00')
  })

  it('creates a supplier, sending the number as digits', async () => {
    let body: unknown
    const { wrapper } = await openFornecedores({}, http.post('*/api/v1/admin/suppliers', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ id: motoPecas.id, name: 'Nova', whatsappPhone: '5592991234567', version: 1 }, { status: 201 })
    }))

    await button(wrapper, 'Novo fornecedor').trigger('click')
    const dialog = wrapper.get('[role=dialog]')
    expect(dialog.get('h2').text()).toBe('Novo fornecedor')
    await field(dialog, 'Nome').setValue('Nova')
    await field(dialog, 'WhatsApp').setValue('(92) 99123-4567')
    await dialog.get('form').trigger('submit')
    await flushPromises()

    expect(body).toEqual({ name: 'Nova', whatsappPhone: '5592991234567' })
    expect(wrapper.find('[role=dialog]').exists()).toBe(false)
    expect(wrapper.text()).toContain('Fornecedor salvo.')
  })

  it('asks to fix a missing name and a wrong number before calling the API', async () => {
    const { wrapper } = await openFornecedores()
    await button(wrapper, 'Novo fornecedor').trigger('click')
    const dialog = wrapper.get('[role=dialog]')
    await field(dialog, 'WhatsApp').setValue('123')

    await dialog.get('form').trigger('submit')
    await flushPromises()

    expect(dialog.text()).toContain('Informe o nome do fornecedor.')
    expect(dialog.text()).toContain('Confira o número: use DDD e 8 ou 9 dígitos.')
    expect(dialog.text()).toContain('Confira os campos destacados.')
  })

  it('saves a change with the version read, and offers "Recarregar" after a conflict', async () => {
    let sentVersion: unknown
    let reads = 0
    const { wrapper } = await openFornecedores({}, http.put(`*/api/v1/admin/suppliers/${couros.id}`, async ({ request }) => {
      sentVersion = ((await request.json()) as { version: number }).version
      return problem(409, 'version_conflict', 'x')
    }), http.get(`*/api/v1/admin/suppliers/${couros.id}`, () => {
      reads += 1
      return HttpResponse.json({ id: couros.id, name: 'Couros Paulista SP', whatsappPhone: couros.phone, version: 4 })
    }))
    await button(wrapper, 'Couros Paulista').trigger('click')
    const dialog = wrapper.get('[role=dialog]')

    await dialog.get('form').trigger('submit')
    await flushPromises()

    expect(sentVersion).toBe(3)
    expect(dialog.text()).toContain('Outra pessoa alterou este fornecedor. Recarregue para ver a versão atual.')
    await button(dialog, 'Recarregar').trigger('click')
    await flushPromises()
    expect(reads).toBe(1)
    expect((field(wrapper.get('[role=dialog]'), 'Nome').element as HTMLInputElement).value).toBe('Couros Paulista SP')
  })

  it('shows a reader the supplier without a way to change it', async () => {
    signedInAs('admin', ['order:read', 'catalog:read', 'purchase:write'])
    const { wrapper } = await openFornecedores()
    expect(wrapper.findAll('button').map((b) => b.text())).not.toContain('Novo fornecedor')

    await button(wrapper, 'Couros Paulista').trigger('click')

    const dialog = wrapper.get('[role=dialog]')
    expect(dialog.findAll('button').map((b) => b.text())).toEqual(['Fechar'])
  })
})

describe('the phone layout', () => {
  // The layout itself is CSS (the Playwright phone project checks 390 px); these are the structures it relies on.
  it('keeps the A comprar boxes as labelled enc-check controls and the actions in the card', async () => {
    const { wrapper } = await openCompras()

    const first = card(wrapper, 'Couros Paulista')
    for (const box of first.findAll('input[type=checkbox]')) {
      expect(box.element.parentElement?.tagName).toBe('LABEL')
      expect(box.element.parentElement?.classList.contains('enc-check')).toBe(true)
    }
    expect(first.findAll('.actions button').map((b) => b.text())).toEqual(['Ver mensagem', 'Enviar pelo WhatsApp'])
  })

  it('turns Enviadas and Fornecedores into cards with labelled cells', async () => {
    use({ orders: [summaryOf(supplierOrderAnswer())] })
    const enviadas = await openCompras('/compras?tab=enviadas', 'C-0012')
    expect(enviadas.wrapper.get('table').classes()).toContain('enc-cards')
    expect(enviadas.wrapper.findAll('tbody tr')[0]?.findAll('td').map((cell) => cell.attributes('data-label'))).toEqual(['Fornecedor', 'Enviado em', 'Prazo do fornecedor', 'Recebidos', 'Situação'])

    use({ suppliers: [{ id: couros.id, name: couros.name, whatsappPhone: couros.phone, version: 1 }] })
    const fornecedores = await openCompras('/compras?tab=fornecedores', 'Couros Paulista')
    expect(fornecedores.wrapper.get('table').classes()).toContain('enc-cards')
    expect(fornecedores.wrapper.findAll('tbody tr')[0]?.findAll('td').map((cell) => cell.attributes('data-label'))).toEqual(['WhatsApp'])
  })

  it('puts "Salvar" and "Cancelar" of the supplier sheet in the sticky footer, a direct child of the form', async () => {
    use({ suppliers: [{ id: couros.id, name: couros.name, whatsappPhone: couros.phone, version: 1 }] })
    const { wrapper } = await openCompras('/compras?tab=fornecedores', 'Couros Paulista')
    await button(wrapper, 'Novo fornecedor').trigger('click')

    expect(wrapper.get('[role=dialog] form > .enc-sheet-actions').findAll('button').map((b) => b.text())).toEqual(['Salvar', 'Cancelar'])
  })
})
