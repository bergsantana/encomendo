import type { FinanceSummary, Permission, TimeZone } from '@encomendo/api'
import { enableAutoUnmount, flushPromises } from '@vue/test-utils'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'

import { button, closeApps, field, link, openApp } from '../../testing/app'
import { authSession, gate, problem } from '../../testing/fixtures'
import { brl, encomendaMontagem, orderSummary, prontaEntrega, settingsUsing } from '../../testing/orders'
import { emptyFinance, todayHandler } from '../../testing/today'

const server = setupServer()
beforeAll(() => server.listen({ onUnhandledFrame: 'error' }))
afterEach(() => {
  server.resetHandlers()
  vi.useRealTimers()
})
afterAll(() => server.close())
enableAutoUnmount(afterEach)
afterEach(closeApps)

const ownerPermissions: Permission[] = ['order:read', 'order:write', 'catalog:read', 'finance:read', 'members:manage', 'settings:write']

const signedInAs = (role: 'owner' | 'admin' | 'staff' | 'viewer', permissions: Permission[]) =>
  server.use(http.post('*/auth/refresh', () => HttpResponse.json(authSession({ role, permissions }))))

const id = (n: number) => `0190f3c4-5e1a-7b2c-8d3e-${String(n).padStart(12, '0')}`
const plain = (text: string) => text.replace(/\s+/g, ' ').trim()
/** line is a table row read cell by cell, as the eye reads it. */
const line = (row: { findAll: (selector: string) => { text: () => string }[] }) => row.findAll('th, td').map((cell) => cell.text()).join(' ').replace(/\s+/g, ' ')

/** november is a period with money in every part of the screen, from the server's own sums. */
const november: FinanceSummary = {
  from: '2026-11-01',
  to: '2026-11-27',
  received: {
    // The total is not the sum of the lines below: it is the server's, and the screen must not recompute it.
    total: brl(1234500),
    byKind: [
      { key: 'deposit', total: brl(800000) },
      { key: 'balance', total: brl(434500) },
    ],
    byMethod: [
      { key: 'pix', total: brl(900000) },
      { key: 'dinheiro', total: brl(100000) },
      { key: 'cartao_credito', total: brl(150000) },
      { key: 'cartao_debito', total: brl(50000) },
      { key: 'transferencia', total: brl(30000) },
      { key: 'outro', total: brl(4500) },
    ],
  },
  toReceive: {
    total: brl(754300),
    orders: 4,
    // By delivery date, as the server sends it; each balance is that order's own.
    list: [
      orderSummary(encomendaMontagem, 'em_montagem', { id: id(211), number: 'P-0231', deliveryDate: '2026-12-02', balanceDue: brl(300000) }),
      orderSummary(prontaEntrega, 'separando', { id: id(212), number: 'P-0198', customer: { id: id(102), name: 'Maria Souza' }, deliveryDate: '2026-12-12', balanceDue: brl(250000) }),
      orderSummary(encomendaMontagem, 'comprando', { id: id(213), number: 'P-0240', deliveryDate: '2026-12-15', balanceDue: brl(150000) }),
      orderSummary(prontaEntrega, 'pagar_sinal', { id: id(214), number: 'P-0250', customer: { id: id(102), name: 'Maria Souza' }, deliveryDate: '2026-12-20', balanceDue: brl(54300) }),
    ],
  },
  toRefund: [
    { order: orderSummary(encomendaMontagem, 'pronto', { id: id(201), number: 'P-0260', customer: { id: id(101), name: 'João Pereira' } }), amount: brl(25050) },
    { order: orderSummary(prontaEntrega, 'separado', { id: id(202), number: 'P-0270', customer: { id: id(102), name: 'Maria Souza' } }), amount: brl(1000) },
  ],
  margins: [
    {
      orderId: id(203),
      number: 'P-0231',
      customerName: 'João Pereira',
      handedOverOn: '2026-11-20',
      total: brl(129990),
      cost: brl(90000),
      margin: { cents: 39990, basisPoints: 3076 },
    },
    { orderId: id(204), number: 'P-0232', customerName: 'Maria Souza', handedOverOn: '2026-11-22', total: brl(50000), cost: null, margin: null },
  ],
  marginsTotal: { total: brl(179990), cost: brl(90000), margin: { cents: 39990, basisPoints: 3076 } },
  truncated: false,
}

interface Seen {
  summaries: URL[]
}

/** serve answers the screen's reads and records each summary query. */
function serve(answer: (url: URL) => FinanceSummary = (url) => ({ ...november, from: url.searchParams.get('from') ?? '', to: url.searchParams.get('to') ?? '' }), timeZone: TimeZone = 'America/Sao_Paulo'): Seen {
  const seen: Seen = { summaries: [] }
  server.use(
    todayHandler(),
    http.get('*/api/v1/admin/settings', () => HttpResponse.json(settingsUsing(encomendaMontagem, { timeZone }))),
    http.get('*/api/v1/admin/finance/summary', ({ request }) => {
      const url = new URL(request.url)
      seen.summaries.push(url)
      return HttpResponse.json(answer(url))
    }),
  )
  return seen
}

/** nowIs fixes the clock without freezing timers, so MSW and the waits keep working. */
const nowIs = (iso: string) => {
  vi.useFakeTimers({ toFake: ['Date'] })
  vi.setSystemTime(new Date(iso))
}

async function openFinanceiro() {
  const app = await openApp('/financeiro')
  await vi.waitFor(() => expect(app.wrapper.text()).toContain('Recebido no período'))
  return app
}

const asked = (url: URL | undefined) => [url?.searchParams.get('from'), url?.searchParams.get('to')]

beforeEach(() => {
  nowIs('2026-11-27T15:00:00Z')
  signedInAs('owner', ownerPermissions)
})

describe('who can open Financeiro', () => {
  it('shows the menu entry to the owner, between Clientes and Equipe', async () => {
    serve()

    const { wrapper } = await openApp('/hoje')

    const entries = wrapper.findAll('nav a.menu-link').map((entry) => entry.text())
    expect(entries).toEqual(['Hoje', 'Pedidos', 'Catálogo', 'Clientes', 'Financeiro', 'Equipe', 'Configurações'])
    expect(link(wrapper, 'Financeiro').attributes('href')).toBe('/financeiro')
  })

  it.each([
    ['admin', ['order:read', 'order:write', 'catalog:read', 'members:manage', 'order:manage', 'customer:write']],
    ['staff', ['order:read', 'order:write', 'catalog:read']],
    ['viewer', ['order:read', 'catalog:read']],
  ] as const)('hides the menu entry from a %s, and the address leads to Hoje without asking the API', async (role, permissions) => {
    const seen = serve()
    signedInAs(role, [...permissions])

    const { wrapper, router } = await openApp('/financeiro')

    expect(router.currentRoute.value.name).toBe('hoje')
    expect(wrapper.findAll('a').map((a) => a.text())).not.toContain('Financeiro')
    expect(wrapper.text()).not.toContain('Recebido no período')
    expect(seen.summaries).toHaveLength(0)
  })

  it('shows the API refusal when a role that the screen let in is refused', async () => {
    serve()
    server.use(http.get('*/api/v1/admin/finance/summary', () => problem(403, 'forbidden', 'Você não tem permissão para isso.')))

    const { wrapper } = await openApp('/financeiro')
    await vi.waitFor(() => expect(wrapper.find('[role=alert]').exists()).toBe(true))

    expect(wrapper.get('[role=alert]').text()).toBe('Você não tem permissão para isso.')
    expect(wrapper.text()).not.toContain('Recebido no período')
  })
})

describe('the period', () => {
  it('starts on "Este mês", from the first of the month to the store day', async () => {
    const seen = serve()

    const { wrapper } = await openFinanceiro()

    expect(asked(seen.summaries[0])).toEqual(['2026-11-01', '2026-11-27'])
    expect((field(wrapper, 'Período').element as HTMLSelectElement).selectedOptions[0]?.text).toBe('Este mês')
    const options = wrapper.findAll('option').map((option) => option.text())
    expect(options).toEqual(['Este mês', 'Mês passado', 'Últimos 30 dias', 'Escolher período'])
  })

  it('sends the whole of last month for "Mês passado"', async () => {
    const seen = serve()
    const { wrapper } = await openFinanceiro()

    await field(wrapper, 'Período').setValue('last_month')
    await flushPromises()

    expect(asked(seen.summaries.at(-1))).toEqual(['2026-10-01', '2026-10-31'])
  })

  it('sends 30 days ending today, both ends counted, for "Últimos 30 dias"', async () => {
    const seen = serve()
    const { wrapper } = await openFinanceiro()

    await field(wrapper, 'Período').setValue('last_30_days')
    await flushPromises()

    expect(asked(seen.summaries.at(-1))).toEqual(['2026-10-29', '2026-11-27'])
  })

  it('reads January as the month after December of the year before', async () => {
    nowIs('2027-01-15T15:00:00Z')
    const seen = serve()
    const { wrapper } = await openFinanceiro()
    expect(asked(seen.summaries[0])).toEqual(['2027-01-01', '2027-01-15'])

    await field(wrapper, 'Período').setValue('last_month')
    await flushPromises()

    expect(asked(seen.summaries.at(-1))).toEqual(['2026-12-01', '2026-12-31'])
  })

  it('shows "De" and "Até" for "Escolher período", reads nothing until a day changes, then sends the days typed', async () => {
    const seen = serve()
    const { wrapper } = await openFinanceiro()
    expect(wrapper.text()).not.toContain('Até')

    await field(wrapper, 'Período').setValue('custom')
    await flushPromises()

    expect(seen.summaries).toHaveLength(1)
    expect((field(wrapper, 'De').element as HTMLInputElement).value).toBe('2026-11-01')
    expect((field(wrapper, 'Até').element as HTMLInputElement).value).toBe('2026-11-27')

    const from = field(wrapper, 'De')
    await from.setValue('2026-10-05')
    await from.trigger('change')
    await flushPromises()
    expect(asked(seen.summaries.at(-1))).toEqual(['2026-10-05', '2026-11-27'])

    const to = field(wrapper, 'Até')
    await to.setValue('2026-10-20')
    await to.trigger('change')
    await flushPromises()
    expect(asked(seen.summaries.at(-1))).toEqual(['2026-10-05', '2026-10-20'])
  })

  it('waits for both days when one of them is cleared', async () => {
    const seen = serve()
    const { wrapper } = await openFinanceiro()
    await field(wrapper, 'Período').setValue('custom')
    const reads = seen.summaries.length

    const from = field(wrapper, 'De')
    await from.setValue('')
    await from.trigger('change')
    await flushPromises()

    expect(seen.summaries).toHaveLength(reads)
  })

  it("takes the store's day, not the viewer's, from the store's time zone", async () => {
    // 03:30 UTC on Dec 1 is still Nov 30 in Manaus (UTC-4) and already Dec 1 in Brasília (UTC-3).
    nowIs('2026-12-01T03:30:00Z')
    const manaus = serve(undefined, 'America/Manaus')
    await openFinanceiro()
    expect(asked(manaus.summaries[0])).toEqual(['2026-11-01', '2026-11-30'])

    closeApps()
    const brasilia = serve(undefined, 'America/Sao_Paulo')
    await openFinanceiro()
    expect(asked(brasilia.summaries[0])).toEqual(['2026-12-01', '2026-12-01'])
  })

  it("still reads the period when the settings can't be read, in the viewer's zone", async () => {
    const seen = serve()
    server.use(http.get('*/api/v1/admin/settings', () => new HttpResponse(null, { status: 500 })))

    const { wrapper } = await openFinanceiro()

    expect(seen.summaries).toHaveLength(1)
    expect(wrapper.find('[role=alert]').exists()).toBe(false)
  })

  it('explains a period the API refuses', async () => {
    serve()
    const { wrapper } = await openFinanceiro()
    server.use(http.get('*/api/v1/admin/finance/summary', () => problem(400, 'invalid', 'Confira os campos.')))

    await field(wrapper, 'Período').setValue('last_month')
    await vi.waitFor(() => expect(wrapper.find('[role=alert]').exists()).toBe(true))

    expect(wrapper.get('[role=alert]').text()).toBe('Confira o período: o início não pode passar do fim, e o período tem no máximo 366 dias.')
    // The figures of the period before never stay under the one that was refused.
    expect(wrapper.text()).not.toContain('Recebido no período')
  })

  it('links the refusal to the fields of "Escolher período"', async () => {
    serve()
    const { wrapper } = await openFinanceiro()
    await field(wrapper, 'Período').setValue('custom')
    server.use(http.get('*/api/v1/admin/finance/summary', () => problem(400, 'invalid', 'Confira os campos.')))

    const to = field(wrapper, 'Até')
    await to.setValue('2026-10-01')
    await vi.waitFor(() => expect(wrapper.text()).toContain('Confira o período'))

    const refused = field(wrapper, 'Até')
    expect(refused.attributes('aria-invalid')).toBe('true')
    const message = wrapper.get(`#${refused.attributes('aria-describedby')}`).text()
    expect(message).toBe('Confira o período: o início não pode passar do fim, e o período tem no máximo 366 dias.')
  })

  it('keeps "Escolher período" when "Tentar de novo" reads again', async () => {
    const seen = serve()
    const { wrapper } = await openFinanceiro()
    await field(wrapper, 'Período').setValue('custom')
    server.use(http.get('*/api/v1/admin/finance/summary', () => new HttpResponse(null, { status: 500 })))
    const to = field(wrapper, 'Até')
    await to.setValue('2026-11-10')
    await vi.waitFor(() => expect(wrapper.text()).toContain('O servidor não respondeu'))

    serve()
    await button(wrapper, 'Tentar de novo').trigger('click')

    await vi.waitFor(() => expect(wrapper.text()).toContain('Recebido no período'))
    expect((field(wrapper, 'Período').element as HTMLSelectElement).value).toBe('custom')
    expect(seen.summaries).toHaveLength(1)
  })

  it('takes the figures of the period before away while the new one loads', async () => {
    serve()
    const { wrapper } = await openFinanceiro()
    const release = gate()
    server.use(
      http.get('*/api/v1/admin/finance/summary', async () => {
        await release.opened
        return HttpResponse.json(november)
      }),
    )

    await field(wrapper, 'Período').setValue('last_month')
    await flushPromises()

    expect(wrapper.text()).toContain('Carregando o financeiro…')
    expect(wrapper.text()).not.toContain('Recebido no período')
    release.open()
    await vi.waitFor(() => expect(wrapper.text()).toContain('Recebido no período'))
  })

  it('shows the answer of the last choice when an older one arrives late', async () => {
    const slow = gate()
    let calls = 0
    server.use(
      todayHandler(),
      http.get('*/api/v1/admin/settings', () => HttpResponse.json(settingsUsing(encomendaMontagem))),
      http.get('*/api/v1/admin/finance/summary', async ({ request }) => {
        calls += 1
        const url = new URL(request.url)
        if (calls === 2) await slow.opened
        return HttpResponse.json({ ...november, toReceive: { total: brl(calls === 2 ? 111100 : 222200), orders: 0, list: [] }, from: url.searchParams.get('from') })
      }),
    )
    const { wrapper } = await openFinanceiro()

    await field(wrapper, 'Período').setValue('last_month')
    await field(wrapper, 'Período').setValue('last_30_days')
    await flushPromises()
    slow.open()
    await flushPromises()

    expect(plain(wrapper.text())).toContain('R$ 2.222,00')
    expect(plain(wrapper.text())).not.toContain('R$ 1.111,00')
  })
})

describe('the summary', () => {
  it('shows what was received, by kind and by method, as the server counted it', async () => {
    serve()

    const { wrapper } = await openFinanceiro()

    const received = plain(wrapper.get('[aria-labelledby=received-title]').text())
    expect(received).toContain('R$ 12.345,00')
    const kinds = wrapper.findAll('[aria-labelledby=received-title] table')[0]?.findAll('tr').map(line)
    expect(kinds).toEqual(['Sinal R$ 8.000,00', 'Saldo R$ 4.345,00'])
    const methods = wrapper.findAll('[aria-labelledby=received-title] table')[1]?.findAll('tr').map(line)
    expect(methods).toEqual([
      'Pix R$ 9.000,00',
      'Dinheiro R$ 1.000,00',
      'Cartão de crédito R$ 1.500,00',
      'Cartão de débito R$ 500,00',
      'Transferência R$ 300,00',
      'Outro R$ 45,00',
    ])
  })

  it('shows a payment kind or method it does not know as the server wrote it', async () => {
    serve((url) => ({
      ...november,
      from: url.searchParams.get('from') ?? '',
      received: { ...november.received, byMethod: [{ key: 'boleto', total: brl(1000) }] },
    }))

    const { wrapper } = await openFinanceiro()

    expect(wrapper.findAll('[aria-labelledby=received-title] tr').map(line)).toEqual(['Sinal R$ 8.000,00', 'Saldo R$ 4.345,00', 'boleto R$ 10,00'])
  })

  it('shows "A receber" with the server total and the number of orders', async () => {
    serve()

    const { wrapper } = await openFinanceiro()

    const section = plain(wrapper.get('[aria-labelledby=to-receive-title]').text())
    expect(section).toContain('A receber')
    expect(section).toContain('R$ 7.543,00')
    expect(section).toContain('4 pedidos com saldo')
  })

  it('lists the orders with a balance, by delivery date, each with its own balance', async () => {
    serve()

    const { wrapper } = await openFinanceiro()

    const section = wrapper.get('[aria-labelledby=to-receive-title]')
    expect(section.findAll('thead th').map((th) => th.text())).toEqual(['Pedido', 'Cliente', 'Saldo', 'Entrega prevista'])
    expect(section.findAll('tbody tr').map(line)).toEqual([
      'P-0231 João Pereira R$ 3.000,00 02/12/2026',
      'P-0198 Maria Souza R$ 2.500,00 12/12/2026',
      'P-0240 João Pereira R$ 1.500,00 15/12/2026',
      'P-0250 Maria Souza R$ 543,00 20/12/2026',
    ])
    // The server's total stays the server's: the balances are shown, never added up here.
    expect(plain(section.get('.total').text())).toBe('R$ 7.543,00')
  })

  it('opens the order from its number in "A receber"', async () => {
    serve()

    const { wrapper } = await openFinanceiro()

    const section = wrapper.get('[aria-labelledby=to-receive-title]')
    expect(link(section, 'P-0198').attributes('href')).toBe(`/pedidos/${id(212)}`)
  })

  it('shows no table when no order has a balance', async () => {
    serve((url) => ({ ...november, from: url.searchParams.get('from') ?? '', toReceive: { total: brl(0), orders: 0, list: [] } }))

    const { wrapper } = await openFinanceiro()

    expect(wrapper.get('[aria-labelledby=to-receive-title]').text()).toContain('0 pedidos com saldo')
    expect(wrapper.find('[aria-labelledby=to-receive-title] table').exists()).toBe(false)
  })

  it('says "1 pedido com saldo" in the singular', async () => {
    serve((url) => ({ ...november, from: url.searchParams.get('from') ?? '', toReceive: { total: brl(5000), orders: 1, list: november.toReceive.list.slice(0, 1) } }))

    const { wrapper } = await openFinanceiro()

    expect(wrapper.get('[aria-labelledby=to-receive-title]').text()).toContain('1 pedido com saldo')
  })

  it('lists "A devolver" with each order in the words of its own workflow', async () => {
    serve()

    const { wrapper } = await openFinanceiro()

    const section = wrapper.get('[aria-labelledby=to-refund-title]')
    expect(section.findAll('thead th').map((th) => th.text())).toEqual(['Pedido', 'Cliente', 'Status', 'Valor a devolver'])
    const rows = section.findAll('tbody tr').map(line)
    expect(rows[0]).toBe('P-0260 João Pereira Pronto R$ 250,50')
    expect(rows[1]).toBe('P-0270 Maria Souza Separado R$ 10,00')
    expect(link(wrapper, 'P-0260').attributes('href')).toBe(`/pedidos/${id(201)}`)
  })

  it('leaves out "A devolver" when nothing is owed', async () => {
    serve((url) => ({ ...november, from: url.searchParams.get('from') ?? '', toRefund: [] }))

    const { wrapper } = await openFinanceiro()

    expect(wrapper.text()).not.toContain('A devolver')
  })

  it('shows each margin with "—" where the cost was not known, and the period total from the server', async () => {
    serve()

    const { wrapper } = await openFinanceiro()

    const section = wrapper.get('[aria-labelledby=margins-title]')
    expect(section.findAll('thead th').map((th) => th.text())).toEqual(['Pedido', 'Entregue em', 'Total', 'Custo', 'Margem'])
    const rows = section.findAll('tbody tr').map(line)
    expect(rows[0]).toContain('P-0231')
    expect(rows[0]).toContain('20/11/2026')
    expect(rows[0]).toContain('R$ 1.299,90')
    expect(rows[0]).toContain('R$ 900,00')
    expect(rows[0]).toContain('R$ 399,90 (30,76%)')
    expect(rows[1]).toContain('P-0232')
    expect(rows[1]).toContain('22/11/2026')
    expect(rows[1]).toContain('R$ 500,00')
    const cells = section.findAll('tbody tr')[1]?.findAll('td').map((cell) => cell.text())
    expect(cells?.slice(-2)).toEqual(['—', '—'])
    // 1.299,90 + 500,00 = 1.799,90: the server's total, with the cost and margin of the orders whose cost was known.
    const footer = plain(section.get('tfoot').text())
    expect(footer).toContain('Total do período')
    expect(footer).toContain('R$ 1.799,90')
    expect(footer).toContain('R$ 900,00')
    expect(footer).toContain('R$ 399,90 (30,76%)')
    expect(section.text()).toContain('Custo e margem somam só os pedidos com todos os custos informados.')
  })

  it('shows "—" for the total margin when the server has none', async () => {
    serve((url) => ({
      ...november,
      from: url.searchParams.get('from') ?? '',
      margins: [november.margins[1] as FinanceSummary['margins'][number]],
      marginsTotal: { total: brl(50000), cost: brl(0), margin: null },
    }))

    const { wrapper } = await openFinanceiro()

    const cells = wrapper.get('[aria-labelledby=margins-title] tfoot').findAll('td').map((cell) => cell.text())
    expect(cells.at(-1)).toBe('—')
  })

  it('links each margin row to its order', async () => {
    serve()

    const { wrapper } = await openFinanceiro()

    expect(link(wrapper.get('[aria-labelledby=margins-title]'), 'P-0231').attributes('href')).toBe(`/pedidos/${id(203)}`)
  })

  it('says so when no order was handed over in the period', async () => {
    serve((url) => ({ ...november, from: url.searchParams.get('from') ?? '', margins: [], marginsTotal: { total: brl(0), cost: brl(0), margin: null } }))

    const { wrapper } = await openFinanceiro()

    expect(wrapper.get('[aria-labelledby=margins-title]').text()).toContain('Nenhum pedido entregue neste período.')
    expect(wrapper.find('tfoot').exists()).toBe(false)
  })

  it('says "Nenhum pagamento neste período." for an empty period', async () => {
    serve((url) => emptyFinance(url.searchParams.get('from') ?? '', url.searchParams.get('to') ?? ''))

    const { wrapper } = await openFinanceiro()

    expect(wrapper.get('[aria-labelledby=received-title]').text()).toContain('Nenhum pagamento neste período.')
    expect(plain(wrapper.get('[aria-labelledby=to-receive-title] .total').text())).toBe('R$ 0,00')
    expect(wrapper.find('[aria-labelledby=to-refund-title]').exists()).toBe(false)
  })

  it('says the figures cover only part of the store when the server flags truncated', async () => {
    serve((url) => ({ ...november, from: url.searchParams.get('from') ?? '', truncated: true }))

    const { wrapper } = await openFinanceiro()

    expect(wrapper.get('[role=status]').text()).toBe(
      'A loja tem mais pedidos do que o Financeiro lê de uma vez. "A receber", "A devolver" e as margens cobrem só parte dos pedidos.',
    )
  })

  it('says nothing about truncation otherwise', async () => {
    serve()

    const { wrapper } = await openFinanceiro()

    expect(wrapper.find('[role=status]').exists()).toBe(false)
  })

  it('says it is loading, then shows the summary', async () => {
    const release = gate()
    server.use(
      todayHandler(),
      http.get('*/api/v1/admin/settings', () => HttpResponse.json(settingsUsing(encomendaMontagem))),
      http.get('*/api/v1/admin/finance/summary', async () => {
        await release.opened
        return HttpResponse.json(november)
      }),
    )

    const { wrapper } = await openApp('/financeiro')
    await vi.waitFor(() => expect(wrapper.text()).toContain('Carregando o financeiro…'))
    expect(wrapper.text()).not.toContain('Recebido no período')

    release.open()
    await vi.waitFor(() => expect(wrapper.text()).toContain('Recebido no período'))
    expect(wrapper.text()).not.toContain('Carregando')
  })

  it('tells the user when the server fails, and reads again on "Tentar de novo"', async () => {
    serve()
    server.use(http.get('*/api/v1/admin/finance/summary', () => new HttpResponse(null, { status: 500 })))
    const { wrapper } = await openApp('/financeiro')
    await vi.waitFor(() => expect(wrapper.find('[role=alert]').exists()).toBe(true))
    expect(wrapper.get('[role=alert]').text()).toBe('O servidor não respondeu. Tente de novo em instantes.')

    serve()
    await button(wrapper, 'Tentar de novo').trigger('click')

    await vi.waitFor(() => expect(wrapper.text()).toContain('Recebido no período'))
    expect(wrapper.find('[role=alert]').exists()).toBe(false)
  })

  it('draws no chart: money is in tables and plain figures', async () => {
    serve()

    const { wrapper } = await openFinanceiro()

    expect(wrapper.find('canvas').exists()).toBe(false)
    expect(wrapper.find('[role=img]').exists()).toBe(false)
  })
})

describe('Financeiro on the phone', () => {
  it('turns "A receber", "A devolver" and the margins into cards, every cell labelled', async () => {
    serve()

    const { wrapper } = await openFinanceiro()

    const tables = wrapper.findAll('table.enc-cards')
    expect(tables).toHaveLength(3)
    const receiveCells = tables[0]?.get('tbody tr').findAll('td').map((cell) => cell.attributes('data-label'))
    expect(receiveCells).toEqual(['Cliente', 'Saldo', 'Entrega prevista'])
    expect(tables[0]?.get('tbody th[scope=row]').text()).toBe('P-0231')
    const refundCells = tables[1]?.get('tbody tr').findAll('td').map((cell) => cell.attributes('data-label'))
    expect(refundCells).toEqual(['Cliente', 'Status', 'Valor a devolver'])
    const marginCells = tables[2]?.get('tbody tr').findAll('td').map((cell) => cell.attributes('data-label'))
    expect(marginCells).toEqual(['Entregue em', 'Total', 'Custo', 'Margem'])
    expect(tables[2]?.get('tfoot').findAll('td[data-label]').map((cell) => cell.attributes('data-label'))).toEqual(['Total', 'Custo', 'Margem'])
  })

  it('keeps the period picker labelled for a thumb: a select and, for a custom period, two day fields', async () => {
    serve()
    const { wrapper } = await openFinanceiro()

    await field(wrapper, 'Período').setValue('custom')

    expect(field(wrapper, 'Período').element.tagName).toBe('SELECT')
    expect(field(wrapper, 'De').attributes('type')).toBe('date')
    expect(field(wrapper, 'Até').attributes('type')).toBe('date')
  })
})
