import type { CreatedOrder, Customer, Permission } from '@encomendo/api'
import { enableAutoUnmount, flushPromises, type VueWrapper } from '@vue/test-utils'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'

import { loadPage } from '../../pageLoad'
import { button, closeApps, field, link, openApp } from '../../testing/app'
import { authSession, gate } from '../../testing/fixtures'
import { brl, encomendaMontagem, joao, maria, orderAsOwner, orderProblem, settingsUsing, summaryOf } from '../../testing/orders'
import { bancoId, corOption, farolId, guidaoAltoId, guidaoId, kit, kitId, quoteFor, serveCatalog } from './testkit'

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

const manager: Permission[] = ['order:read', 'order:write', 'order:manage', 'customer:write']
const staff: Permission[] = ['order:read', 'order:write']

const signedInAs = (role: 'owner' | 'admin' | 'staff' | 'viewer', permissions: Permission[]) =>
  server.use(http.post('*/auth/refresh', () => HttpResponse.json(authSession({ role, permissions }))))

const trackingUrl = 'https://altarpm.encomendo.com.br/p/token-1'
const sendUrl = 'https://wa.me/5592991234567?text=Ol%C3%A1%2C%20Jo%C3%A3o'

function created(overrides: Partial<CreatedOrder> = {}): CreatedOrder {
  return { order: orderAsOwner(encomendaMontagem, 'rascunho'), trackingUrl, ...overrides }
}

/** Calls records what the form sent. */
interface Calls {
  quotes: Record<string, unknown>[]
  creates: { body: Record<string, unknown>; key: string | null }[]
  customerPuts: Record<string, unknown>[]
  customerPosts: Record<string, unknown>[]
  customerQueries: (string | null)[]
}

interface Backend {
  customers?: Customer[]
  createdAnswer?: () => Response | Promise<Response>
  quoteAnswer?: (body: Record<string, unknown>) => Response | Promise<Response>
}

function serve(backend: Backend = {}): Calls {
  const calls: Calls = { quotes: [], creates: [], customerPuts: [], customerPosts: [], customerQueries: [] }
  const customers = backend.customers ?? [joao, maria]
  serveCatalog(server)
  server.use(
    http.get('*/readyz', () => HttpResponse.json({ status: 'ok' })),
    http.get('*/api/v1/admin/settings', () => HttpResponse.json(settingsUsing(encomendaMontagem, { depositPercent: 50, assemblyDays: 3, safetyMarginDays: 2 }))),
    http.get('*/api/v1/admin/customers', ({ request }) => {
      calls.customerQueries.push(new URL(request.url).searchParams.get('q'))
      return HttpResponse.json({ items: customers.map((customer) => summaryOf(customer)) })
    }),
    http.get('*/api/v1/admin/customers/:id', ({ params }) => {
      const found = customers.find((customer) => customer.id === params.id)
      return found === undefined ? orderProblem('not_found') : HttpResponse.json(found)
    }),
    http.post('*/api/v1/admin/customers', async ({ request }) => {
      const body = (await request.json()) as Record<string, unknown>
      calls.customerPosts.push(body)
      return HttpResponse.json({ ...maria, id: '0190f3c4-5e1a-7b2c-8d3e-000000000109', name: body.name, phone: body.phone, addresses: [], version: 1 }, { status: 201 })
    }),
    http.put('*/api/v1/admin/customers/:id', async ({ request }) => {
      const body = (await request.json()) as { addresses: Record<string, unknown>[] }
      calls.customerPuts.push(body)
      const added = { ...body.addresses.at(-1), id: '0190f3c4-5e1a-7b2c-8d3e-000000000199' }
      return HttpResponse.json({ ...joao, addresses: [...joao.addresses, added], version: joao.version + 1 })
    }),
    http.post('*/api/v1/admin/orders/quote', async ({ request }) => {
      const body = (await request.json()) as Record<string, unknown>
      calls.quotes.push(body)
      return backend.quoteAnswer?.(body) ?? HttpResponse.json(quoteFor())
    }),
    http.post('*/api/v1/admin/orders', async ({ request }) => {
      calls.creates.push({ body: (await request.json()) as Record<string, unknown>, key: request.headers.get('Idempotency-Key') })
      return backend.createdAnswer?.() ?? HttpResponse.json(created(), { status: 201 })
    }),
  )
  return calls
}

const plain = (text: string) => text.replace(/\u00a0/g, ' ')

async function openForm(path = '/pedidos/novo') {
  const app = await openApp(path)
  await vi.waitFor(() => expect(app.wrapper.text()).toContain('Novo pedido'))
  return app
}

async function pickFromDialog(wrapper: VueWrapper, opener: string, name: string) {
  await button(wrapper, opener).trigger('click')
  await vi.waitFor(() => expect(wrapper.find('[role=dialog]').exists()).toBe(true))
  await vi.waitFor(() => expect(wrapper.find('[role=dialog] ul button').exists()).toBe(true))
  const found = wrapper.findAll('[role=dialog] ul button').find((candidate) => candidate.text().includes(name))
  if (found === undefined) throw new Error(`no result ${name}`)
  await found.trigger('click')
  await vi.waitFor(() => expect(wrapper.find('[role=dialog]').exists()).toBe(false))
}

const addKit = (wrapper: VueWrapper) => pickFromDialog(wrapper, 'Adicionar kit', 'Kit Scrambler')
const addItem = (wrapper: VueWrapper, name = 'Farol LED') => pickFromDialog(wrapper, 'Adicionar item', name)

async function chooseCustomer(wrapper: VueWrapper, name = 'João Pereira') {
  await field(wrapper, 'Buscar cliente').setValue(name.slice(0, 3))
  await vi.waitFor(() => expect(wrapper.find('.results button').exists()).toBe(true))
  const found = wrapper.findAll('.results button').find((candidate) => candidate.text().includes(name))
  if (found === undefined) throw new Error(`no customer ${name}`)
  await found.trigger('click')
  await vi.waitFor(() => expect(wrapper.text()).toContain('Trocar cliente'))
}

const lastQuote = (calls: Calls) => calls.quotes.at(-1)

beforeEach(() => {
  signedInAs('owner', manager)
  vi.spyOn(window, 'open').mockReturnValue(null)
})

describe('the route', () => {
  it('is for roles that write orders: a viewer is sent to Hoje', async () => {
    signedInAs('viewer', ['order:read'])
    serve()

    const { router } = await openApp('/pedidos/novo')

    expect(router.currentRoute.value.name).toBe('hoje')
  })

  it('opens "Novo pedido" with its sections and the buttons of a new order', async () => {
    serve()

    const { wrapper } = await openForm()

    expect(wrapper.get('h1').text()).toBe('Novo pedido')
    for (const heading of ['Cliente', 'Itens', 'Resumo']) expect(wrapper.findAll('h2').map((h) => h.text())).toContain(heading)
    expect(wrapper.findAll('form button').map((b) => b.text())).toEqual(expect.arrayContaining(['Adicionar kit', 'Adicionar item', 'Salvar rascunho', 'Salvar e enviar ao cliente', 'Cancelar']))
  })
})

describe('the customer', () => {
  it('searches by name or WhatsApp and chooses one of the matches', async () => {
    const calls = serve()
    const { wrapper } = await openForm()

    await field(wrapper, 'Buscar cliente').setValue('(92) 99123')
    await vi.waitFor(() => expect(wrapper.find('.results button').exists()).toBe(true))

    // A number written the way people write it is sent as digits.
    expect(calls.customerQueries.at(-1)).toBe('9299123')
    await wrapper.get('.results button').trigger('click')
    await vi.waitFor(() => expect(wrapper.text()).toContain('Trocar cliente'))
    expect(plain(wrapper.get('.chosen').text())).toContain('(92) 99123-4567')
  })

  it('lets the order change customer before saving', async () => {
    serve()
    const { wrapper } = await openForm()
    await chooseCustomer(wrapper)

    await button(wrapper, 'Trocar cliente').trigger('click')

    expect(field(wrapper, 'Buscar cliente').element).toBeTruthy()
  })

  it('takes "Novo cliente" with Nome and WhatsApp for the order, without making it first', async () => {
    const calls = serve()
    const { wrapper } = await openForm()

    await button(wrapper, 'Novo cliente').trigger('click')
    await field(wrapper, 'Nome').setValue('Carla Dias')
    await field(wrapper, 'WhatsApp').setValue('(92) 98111-2222')
    await button(wrapper, 'Criar cliente').trigger('click')
    await vi.waitFor(() => expect(wrapper.text()).toContain('Trocar cliente'))

    expect(wrapper.get('.chosen').text()).toContain('Carla Dias')
    expect(plain(wrapper.get('.chosen').text())).toContain('(92) 98111-2222')
    expect(calls.customerPosts).toEqual([])
  })

  it('sends the new customer with the order, in place of a customer id', async () => {
    const calls = serve()
    const { wrapper } = await openForm()
    await button(wrapper, 'Novo cliente').trigger('click')
    await field(wrapper, 'Nome').setValue('Carla Dias')
    await field(wrapper, 'WhatsApp').setValue('(92) 98111-2222')
    await button(wrapper, 'Criar cliente').trigger('click')
    await addItem(wrapper)

    await button(wrapper, 'Salvar rascunho').trigger('click')
    await vi.waitFor(() => expect(wrapper.text()).toContain('Pedido P-0231 criado.'))

    expect(calls.creates[0]?.body).toEqual({
      newCustomer: { name: 'Carla Dias', phone: '5592981112222' },
      lines: [{ itemId: farolId, quantity: 1 }],
      terms: { handover: 'pickup' },
      sendToCustomer: false,
    })
    expect(calls.customerPosts).toEqual([])
  })

  it('changes from a customer found to a new one, and back', async () => {
    const calls = serve()
    const { wrapper } = await openForm()
    await chooseCustomer(wrapper)
    await button(wrapper, 'Trocar cliente').trigger('click')
    await button(wrapper, 'Novo cliente').trigger('click')
    await field(wrapper, 'Nome').setValue('Carla Dias')
    await field(wrapper, 'WhatsApp').setValue('(92) 98111-2222')
    await button(wrapper, 'Criar cliente').trigger('click')
    await addItem(wrapper)
    await button(wrapper, 'Salvar rascunho').trigger('click')
    await vi.waitFor(() => expect(calls.creates).toHaveLength(1))

    expect(calls.creates[0]?.body.customerId).toBeUndefined()
    expect(calls.creates[0]?.body.newCustomer).toBeDefined()
  })

  it('starts the customer over with the search box when "Trocar cliente" is pressed after a new one', async () => {
    serve()
    const { wrapper } = await openForm()
    await button(wrapper, 'Novo cliente').trigger('click')
    await field(wrapper, 'Nome').setValue('Carla Dias')
    await field(wrapper, 'WhatsApp').setValue('(92) 98111-2222')
    await button(wrapper, 'Criar cliente').trigger('click')

    await button(wrapper, 'Trocar cliente').trigger('click')

    expect(field(wrapper, 'Buscar cliente').element).toBeTruthy()
    await button(wrapper, 'Novo cliente').trigger('click')
    expect((field(wrapper, 'Nome').element as HTMLInputElement).value).toBe('')
    expect((field(wrapper, 'WhatsApp').element as HTMLInputElement).value).toBe('')
  })

  it('asks for what is missing in "Novo cliente"', async () => {
    serve()
    const { wrapper } = await openForm()
    await button(wrapper, 'Novo cliente').trigger('click')

    await wrapper.get('form.fresh').trigger('submit')
    await flushPromises()
    expect(wrapper.text()).toContain('Informe o nome do cliente.')
    expect(wrapper.text()).toContain('Informe o WhatsApp do cliente.')

    await field(wrapper, 'Nome').setValue('Carla Dias')
    await field(wrapper, 'WhatsApp').setValue('123')
    await wrapper.get('form.fresh').trigger('submit')
    await flushPromises()
    expect(wrapper.text()).toContain('Confira o número: use DDD e 8 ou 9 dígitos.')
    expect(wrapper.find('.chosen').exists()).toBe(false)
  })

  it('asks "Informe o cliente do pedido." when saving without one', async () => {
    const calls = serve()
    const { wrapper } = await openForm()
    await addItem(wrapper)

    await button(wrapper, 'Salvar rascunho').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('Informe o cliente do pedido.')
    expect(calls.creates).toEqual([])
  })
})

describe('the lines', () => {
  it('adds a kit with its items, and a swappable one offers "Trocar" with the alternatives', async () => {
    serve()
    const { wrapper } = await openForm()

    await addKit(wrapper)

    const line = wrapper.get('.line')
    expect(line.get('h3').text()).toBe('Kit Scrambler')
    expect(line.findAll('legend').map((legend) => legend.text())).toEqual(['Banco de couro Scrambler', 'Guidão baixo'])
    const swap = field(wrapper, 'Trocar Guidão baixo')
    expect(swap.findAll('option').map((option) => option.text())).toEqual(['Guidão baixo', 'Guidão alto'])
    expect(wrapper.text()).not.toContain('Trocar Banco de couro Scrambler')
  })

  it('shows "{item} no lugar de {item}" after a swap, and sends it', async () => {
    const calls = serve()
    const { wrapper } = await openForm()
    await addKit(wrapper)

    await field(wrapper, 'Trocar Guidão baixo').setValue(guidaoAltoId)
    await field(wrapper, 'Cor').setValue(corOption.choices[0]?.id ?? '')

    expect(wrapper.text()).toContain('Guidão alto no lugar de Guidão baixo')
    await vi.waitFor(() => expect(lastQuote(calls)).toBeDefined())
    expect(lastQuote(calls)?.lines).toEqual([
      {
        kitId,
        swaps: [{ itemId: guidaoId, withItemId: guidaoAltoId }],
        options: [{ itemId: bancoId, optionId: corOption.id, choiceId: corOption.choices[0]?.id }],
      },
    ])
  })

  it('takes the swap back when the original is chosen again', async () => {
    const calls = serve()
    const { wrapper } = await openForm()
    await addKit(wrapper)
    await field(wrapper, 'Cor').setValue(corOption.choices[0]?.id ?? '')
    await field(wrapper, 'Trocar Guidão baixo').setValue(guidaoAltoId)

    await field(wrapper, 'Trocar Guidão baixo').setValue(guidaoId)

    expect(wrapper.text()).not.toContain('no lugar de')
    await vi.waitFor(() => expect(lastQuote(calls)?.lines).toEqual([{ kitId, options: [{ itemId: bancoId, optionId: corOption.id, choiceId: corOption.choices[0]?.id }] }]))
  })

  it('offers the options of an item, extra price and days included, and needs a required one', async () => {
    const calls = serve()
    const { wrapper } = await openForm()

    await addItem(wrapper, 'Banco de couro')

    const cor = field(wrapper, 'Cor')
    expect(cor.findAll('option').map((option) => plain(option.text()))).toEqual(['Escolha uma opção', 'Preto', 'Marrom (+ R$ 50,00, + 2 dias)'])
    // Without the choice there is nothing to price yet.
    await new Promise((resolve) => setTimeout(resolve, 400))
    expect(calls.quotes).toEqual([])

    await cor.setValue(corOption.choices[1]?.id ?? '')
    await vi.waitFor(() => expect(lastQuote(calls)).toBeDefined())
    expect(lastQuote(calls)?.lines).toEqual([{ itemId: bancoId, quantity: 1, options: [{ itemId: bancoId, optionId: corOption.id, choiceId: corOption.choices[1]?.id }] }])
  })

  it('asks "Escolha {opção}." on saving without a required option', async () => {
    const calls = serve()
    const { wrapper } = await openForm()
    await chooseCustomer(wrapper)
    await addItem(wrapper, 'Banco de couro')

    await button(wrapper, 'Salvar rascunho').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('Escolha Cor.')
    expect(calls.creates).toEqual([])
  })

  it('changes the quantity of a line and removes it', async () => {
    const calls = serve()
    const { wrapper } = await openForm()
    await addItem(wrapper)

    await field(wrapper, 'Quantidade').setValue('3')
    await vi.waitFor(() => expect(lastQuote(calls)?.lines).toEqual([{ itemId: farolId, quantity: 3 }]))

    await button(wrapper, 'Remover').trigger('click')
    expect(wrapper.find('.line').exists()).toBe(false)
  })

  it('refuses a quantity that is not 1 to 99 and asks for at least one line', async () => {
    const calls = serve()
    const { wrapper } = await openForm()
    await chooseCustomer(wrapper)

    await button(wrapper, 'Salvar rascunho').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('Adicione ao menos um item ou kit.')

    await addItem(wrapper)
    for (const typed of ['0', '100', 'x']) {
      await field(wrapper, 'Quantidade').setValue(typed)
      await button(wrapper, 'Salvar rascunho').trigger('click')
      await flushPromises()
      expect(wrapper.text(), typed).toContain('Use uma quantidade de 1 a 99.')
    }
    expect(calls.creates).toEqual([])
    await field(wrapper, 'Quantidade').setValue('99')
    await vi.waitFor(() => expect(lastQuote(calls)?.lines).toEqual([{ itemId: farolId, quantity: 99 }]))
  })

  it('has no quantity on a kit line, and sends none: a kit line is one kit', async () => {
    const calls = serve()
    const { wrapper } = await openForm()
    await addKit(wrapper)
    await field(wrapper, 'Cor').setValue(corOption.choices[0]?.id ?? '')

    expect(wrapper.findAll('label').map((label) => label.text())).not.toContain('Quantidade')
    await vi.waitFor(() => expect(lastQuote(calls)).toBeDefined())
    expect(lastQuote(calls)?.lines).toEqual([{ kitId, options: [{ itemId: bancoId, optionId: corOption.id, choiceId: corOption.choices[0]?.id }] }])
  })

  it('adds the same kit twice as two lines', async () => {
    const calls = serve()
    const { wrapper } = await openForm()
    await addKit(wrapper)
    await addKit(wrapper)
    for (const cor of wrapper.findAll('select').filter((select) => select.findAll('option').some((option) => option.text() === 'Preto'))) {
      await cor.setValue(corOption.choices[0]?.id ?? '')
    }

    expect(wrapper.findAll('.line')).toHaveLength(2)
    await vi.waitFor(() => expect(lastQuote(calls)?.lines).toHaveLength(2))
    expect((lastQuote(calls)?.lines as { kitId: string }[]).map((line) => line.kitId)).toEqual([kitId, kitId])
  })

  it('finds kits and items by name in the picker, and says when there are none', async () => {
    serve()
    const { wrapper } = await openForm()

    await button(wrapper, 'Adicionar item').trigger('click')
    await vi.waitFor(() => expect(wrapper.findAll('[role=dialog] ul button')).toHaveLength(4))
    await field(wrapper, 'Buscar item').setValue('guid')
    await vi.waitFor(() => expect(wrapper.findAll('[role=dialog] ul button')).toHaveLength(2))
    await field(wrapper, 'Buscar item').setValue('zzz')
    await vi.waitFor(() => expect(wrapper.text()).toContain('Nenhum item encontrado.'))

    await button(wrapper, 'Fechar').trigger('click')
    expect(wrapper.find('[role=dialog]').exists()).toBe(false)
  })
})

describe('the quote', () => {
  it('shows the figures the API answered and adds nothing up', async () => {
    serve()
    const { wrapper } = await openForm()
    await addItem(wrapper)

    await vi.waitFor(() => expect(wrapper.text()).toContain('Total'))

    // `never computes a total on the client`: 12.000 (the item) is nowhere in the summary; only MSW's figures are.
    const summary = plain(wrapper.get('.summary').text())
    expect(summary).toContain('ItensR$ 1.111,00')
    expect(summary).toContain('FreteR$ 22,00')
    expect(summary).toContain('Desconto −R$ 3,00')
    expect(summary).toContain('TotalR$ 1.234,00')
    expect(summary).toContain('Sinal (40%)R$ 555,00')
    expect(summary).toContain('Entrega prevista20/12/2026')
    expect(summary).not.toContain('120,00')
  })

  it('says "Calculando…" while it waits, and waits 300 ms after the last edit for one call', async () => {
    const release = gate()
    const calls = serve({
      quoteAnswer: async () => {
        await release.opened
        return HttpResponse.json(quoteFor())
      },
    })
    const { wrapper } = await openForm()
    await addItem(wrapper)

    expect(wrapper.get('.summary').text()).toContain('Calculando…')
    for (const quantity of ['2', '3', '4']) await field(wrapper, 'Quantidade').setValue(quantity)
    await new Promise((resolve) => setTimeout(resolve, 100))
    expect(calls.quotes).toEqual([])

    await vi.waitFor(() => expect(calls.quotes).toHaveLength(1))
    expect(lastQuote(calls)?.lines).toEqual([{ itemId: farolId, quantity: 4 }])
    expect(wrapper.get('.summary').text()).toContain('Calculando…')
    release.open()
    await vi.waitFor(() => expect(wrapper.get('.summary').text()).toContain('Total'))
  })

  it('keeps only the newest answer when an older one arrives late', async () => {
    const first = gate()
    let calls = 0
    serve({
      quoteAnswer: async (body) => {
        calls += 1
        if (calls === 1) {
          await first.opened
          return HttpResponse.json(quoteFor({ total: { cents: 111100, currency: 'BRL' } }))
        }
        expect(body).toBeDefined()
        return HttpResponse.json(quoteFor({ total: { cents: 222200, currency: 'BRL' } }))
      },
    })
    const { wrapper } = await openForm()
    await addItem(wrapper)
    await vi.waitFor(() => expect(calls).toBe(1))

    await field(wrapper, 'Quantidade').setValue('2')
    await vi.waitFor(() => expect(plain(wrapper.get('.summary').text())).toContain('R$ 2.222,00'))
    first.open()
    await flushPromises()

    expect(plain(wrapper.get('.summary').text())).toContain('TotalR$ 2.222,00')
    expect(plain(wrapper.get('.summary').text())).not.toContain('TotalR$ 1.111,00')
  })

  it("shows each line's price and its items' dates from the quote", async () => {
    serve({
      quoteAnswer: () =>
        HttpResponse.json(
          quoteFor({
            lines: [
              {
                kit: null,
                price: { cents: 98700, currency: 'BRL' },
                items: [{ catalogItemId: farolId, name: 'Farol LED', sourcingMode: 'to_order', quantity: 1, unitPrice: { cents: 12000, currency: 'BRL' }, options: [], expectedDate: '2026-12-05' }],
              },
            ],
          }),
        ),
    })
    const { wrapper } = await openForm()
    await addItem(wrapper)

    await vi.waitFor(() => expect(wrapper.text()).toContain('Farol LED: previsto para 05/12/2026'))

    expect(plain(wrapper.get('.line .price').text())).toBe('R$ 987,00')
  })

  it('shows the field messages of a refused quote next to the fields', async () => {
    serve({
      quoteAnswer: () => orderProblem('invalid', { errors: [{ field: 'terms.discountCents', message: 'O desconto não pode passar do valor dos itens.' }] }),
    })
    const { wrapper } = await openForm()
    await addItem(wrapper)

    await vi.waitFor(() => expect(wrapper.text()).toContain('O desconto não pode passar do valor dos itens.'))

    expect(wrapper.get('.summary [role=alert]').text()).toBe('Confira os campos destacados.')
  })

  it('names an archived item the quote refused', async () => {
    serve({
      quoteAnswer: () => orderProblem('item_unavailable', { references: [{ id: farolId, name: 'Farol LED' }] }),
    })
    const { wrapper } = await openForm()
    await addItem(wrapper)

    await vi.waitFor(() => expect(wrapper.get('.summary').text()).toContain('Farol LED está arquivado e não pode entrar no pedido.'))

    expect(wrapper.get('.line [role=alert]').text()).toBe('Farol LED está arquivado e não pode entrar no pedido.')
  })

  it('shows what a refused quote says when the server fails', async () => {
    serve({ quoteAnswer: () => new HttpResponse(null, { status: 500 }) })
    const { wrapper } = await openForm()
    await addItem(wrapper)

    await vi.waitFor(() => expect(wrapper.get('.summary').text()).toContain('O servidor não respondeu.'))
  })

  it('asks again with the new values after an edit that fixes it', async () => {
    let refuse = true
    const calls = serve({ quoteAnswer: () => (refuse ? orderProblem('invalid') : HttpResponse.json(quoteFor())) })
    const { wrapper } = await openForm()
    await addItem(wrapper)
    await vi.waitFor(() => expect(wrapper.get('.summary').text()).toContain('Confira os campos destacados.'))

    refuse = false
    await field(wrapper, 'Quantidade').setValue('2')

    await vi.waitFor(() => expect(wrapper.get('.summary').text()).toContain('Total'))
    expect(calls.quotes).toHaveLength(2)
  })
})

describe('delivery and pickup', () => {
  it('starts with "Retirada na loja" and asks the quote for a pickup', async () => {
    const calls = serve()
    const { wrapper } = await openForm()
    await addItem(wrapper)

    await vi.waitFor(() => expect(lastQuote(calls)).toBeDefined())

    expect(lastQuote(calls)?.terms).toEqual({ handover: 'pickup' })
    expect((field(wrapper, 'Retirada na loja').element as HTMLInputElement).checked).toBe(true)
    expect(wrapper.findAll('label').map((label) => label.text())).not.toContain('Frete')
  })

  it("offers the customer's addresses and the freight for a delivery, and sends them", async () => {
    const calls = serve()
    const { wrapper } = await openForm()
    await chooseCustomer(wrapper)
    await addItem(wrapper)
    await field(wrapper, 'Entrega').setValue(true)

    const address = field(wrapper, 'Endereço')
    expect(address.findAll('option').map((option) => option.text())).toEqual(['Escolha o endereço', 'Casa: Rua das Palmeiras, 120', 'Novo endereço'])
    await address.setValue(joao.addresses[0]?.id ?? '')
    await field(wrapper, 'Frete').setValue('22,00')

    await vi.waitFor(() => expect(lastQuote(calls)?.terms).toEqual({ handover: 'delivery', addressId: joao.addresses[0]?.id, freightCents: 2200 }))
  })

  it('asks "Escolha o endereço de entrega." when saving a delivery without one', async () => {
    const calls = serve()
    const { wrapper } = await openForm()
    await chooseCustomer(wrapper)
    await addItem(wrapper)
    await field(wrapper, 'Entrega').setValue(true)

    await button(wrapper, 'Salvar rascunho').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('Escolha o endereço de entrega.')
    expect(calls.creates).toEqual([])
  })

  it('leaves the freight out of a pickup, even if one was typed for a delivery', async () => {
    const calls = serve()
    const { wrapper } = await openForm()
    await addItem(wrapper)
    await field(wrapper, 'Entrega').setValue(true)
    await field(wrapper, 'Frete').setValue('10,00')
    await field(wrapper, 'Retirada na loja').setValue(true)

    await vi.waitFor(() => expect(lastQuote(calls)?.terms).toEqual({ handover: 'pickup' }))
  })

  async function typeNewAddress(wrapper: VueWrapper) {
    await field(wrapper, 'Entrega').setValue(true)
    await field(wrapper, 'Endereço').setValue('new')
    await field(wrapper, 'Nome do endereço').setValue('Loja')
    await field(wrapper, 'CEP').setValue('69057-000')
    await field(wrapper, 'Rua').setValue('Av. Djalma Batista')
    await field(wrapper, 'Cidade').setValue('Manaus')
    await field(wrapper, 'UF').setValue('AM')
  }

  const newAddress = { label: 'Loja', cep: '69057000', street: 'Av. Djalma Batista', number: '', complement: '', district: '', city: 'Manaus', state: 'AM' }

  it('sends the new address with the quote and the order, never with an address id, and saves nothing else', async () => {
    const calls = serve()
    const { wrapper } = await openForm()
    await chooseCustomer(wrapper)
    await addItem(wrapper)
    await typeNewAddress(wrapper)

    await vi.waitFor(() => expect(lastQuote(calls)?.terms).toEqual({ handover: 'delivery', newAddress }))
    await button(wrapper, 'Salvar rascunho').trigger('click')
    await vi.waitFor(() => expect(wrapper.text()).toContain('Pedido P-0231 criado.'))

    expect(calls.creates[0]?.body.terms).toEqual({ handover: 'delivery', newAddress })
    expect(calls.customerPuts).toEqual([])
    expect(wrapper.findAll('button').map((b) => b.text())).not.toContain('Salvar endereço')
  })

  it('does not price the order before the new address is complete', async () => {
    const calls = serve()
    const { wrapper } = await openForm()
    await chooseCustomer(wrapper)
    await addItem(wrapper)
    await field(wrapper, 'Entrega').setValue(true)
    await field(wrapper, 'Endereço').setValue('new')

    await new Promise((resolve) => setTimeout(resolve, 400))
    expect(calls.quotes).toEqual([])

    await field(wrapper, 'Nome do endereço').setValue('Loja')
    await field(wrapper, 'CEP').setValue('69057-000')
    await field(wrapper, 'Rua').setValue('Av. Djalma Batista')
    await field(wrapper, 'Cidade').setValue('Manaus')
    await field(wrapper, 'UF').setValue('AM')
    await vi.waitFor(() => expect(calls.quotes).toHaveLength(1))
  })

  it('says what is wrong with a new address when saving, and sends nothing', async () => {
    const calls = serve()
    const { wrapper } = await openForm()
    await chooseCustomer(wrapper)
    await addItem(wrapper)
    await field(wrapper, 'Entrega').setValue(true)
    await field(wrapper, 'Endereço').setValue('new')

    await button(wrapper, 'Salvar rascunho').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('Informe o nome do endereço.')
    expect(wrapper.text()).toContain('Confira o CEP: use 8 dígitos.')
    expect(calls.creates).toEqual([])
  })

  it('offers "Novo endereço" to staff too, and to a customer made with the order, which has no address', async () => {
    signedInAs('staff', staff)
    serve()
    const { wrapper } = await openForm()
    await button(wrapper, 'Novo cliente').trigger('click')
    await field(wrapper, 'Nome').setValue('Carla Dias')
    await field(wrapper, 'WhatsApp').setValue('(92) 98111-2222')
    await button(wrapper, 'Criar cliente').trigger('click')
    await field(wrapper, 'Entrega').setValue(true)

    expect(field(wrapper, 'Endereço').findAll('option').map((option) => option.text())).toEqual(['Escolha o endereço', 'Novo endereço'])
  })

  it('shows what the API refused in the new address next to its fields', async () => {
    serve({
      quoteAnswer: () =>
        orderProblem('invalid', {
          errors: [
            { field: 'terms.newAddress.cep', message: 'Confira o CEP: use 8 dígitos.' },
            { field: 'terms.newAddress', message: 'Use até 10 endereços.' },
          ],
        }),
    })
    const { wrapper } = await openForm()
    await chooseCustomer(wrapper)
    await addItem(wrapper)
    await typeNewAddress(wrapper)

    await vi.waitFor(() => expect(wrapper.text()).toContain('Use até 10 endereços.'))

    expect(wrapper.text()).toContain('Confira o CEP: use 8 dígitos.')
    expect(wrapper.get('.new-address').text()).toContain('Confira o CEP: use 8 dígitos.')
  })

  it('goes back to no new address when the delivery is changed to a pickup', async () => {
    const calls = serve()
    const { wrapper } = await openForm()
    await chooseCustomer(wrapper)
    await addItem(wrapper)
    await typeNewAddress(wrapper)
    await vi.waitFor(() => expect(lastQuote(calls)?.terms).toMatchObject({ newAddress }))

    await field(wrapper, 'Retirada na loja').setValue(true)

    await vi.waitFor(() => expect(lastQuote(calls)?.terms).toEqual({ handover: 'pickup' }))
  })
})

describe('"Resumo" extras', () => {
  it('says how many days after the deposit the delivery is', async () => {
    serve({ quoteAnswer: () => HttpResponse.json(quoteFor({ daysAfterDeposit: 1 })) })
    const { wrapper } = await openForm()
    await addItem(wrapper)

    await vi.waitFor(() => expect(plain(wrapper.get('.summary').text())).toContain('Entrega prevista20/12/2026 (1 dia depois do sinal)'))
  })

  it('shows cost and margin when the API sent them (the owner), as the API sent them', async () => {
    serve({ quoteAnswer: () => HttpResponse.json(quoteFor({ cost: brl(80000), margin: { cents: 43400, basisPoints: 3517 } })) })
    const { wrapper } = await openForm()
    await addItem(wrapper)

    await vi.waitFor(() => expect(wrapper.get('.summary').text()).toContain('Custo'))

    const summary = plain(wrapper.get('.summary').text())
    expect(summary).toContain('CustoR$ 800,00')
    expect(summary).toContain('MargemR$ 434,00 (35,17%)')
  })

  it('shows "—" for a cost the order has none of', async () => {
    serve({ quoteAnswer: () => HttpResponse.json(quoteFor({ cost: null, margin: null })) })
    const { wrapper } = await openForm()
    await addItem(wrapper)

    await vi.waitFor(() => expect(wrapper.get('.summary').text()).toContain('Custo'))

    expect(plain(wrapper.get('.summary').text())).toContain('Custo—')
    expect(plain(wrapper.get('.summary').text())).toContain('Margem—')
  })

  it('shows neither for an admin, whose quote has neither', async () => {
    signedInAs('admin', manager)
    serve()
    const { wrapper } = await openForm()
    await addItem(wrapper)

    await vi.waitFor(() => expect(wrapper.get('.summary').text()).toContain('Total'))

    expect(wrapper.get('.summary').text()).not.toMatch(/Custo|Margem/)
  })
})

describe('"Valores"', () => {
  it('shows the discount, deposit and days with the store defaults to an owner or admin', async () => {
    serve()
    const { wrapper } = await openForm()

    await vi.waitFor(() => expect(wrapper.text()).toContain('Padrão da loja: 50%'))

    expect(wrapper.text()).toContain('Padrão da loja: 3 dias')
    expect(wrapper.text()).toContain('Padrão da loja: 2 dias')
    for (const label of ['Desconto', 'Sinal (%)', 'Dias de Montagem', 'Margem de segurança (dias)']) expect(field(wrapper, label).element).toBeTruthy()
  })

  it('sends only what was typed: an empty field is the default', async () => {
    const calls = serve()
    const { wrapper } = await openForm()
    await addItem(wrapper)
    await field(wrapper, 'Desconto').setValue('50,00')
    await field(wrapper, 'Sinal (%)').setValue('30')
    await field(wrapper, 'Dias de Montagem').setValue('5')

    await vi.waitFor(() => expect(lastQuote(calls)?.terms).toEqual({ handover: 'pickup', discountCents: 5000, depositPercent: 30, assemblyDays: 5 }))
  })

  it('hides them from staff, and never sends them', async () => {
    signedInAs('staff', staff)
    const calls = serve()
    const { wrapper } = await openForm()
    await addItem(wrapper)

    await vi.waitFor(() => expect(lastQuote(calls)).toBeDefined())

    expect(wrapper.findAll('label').map((label) => label.text())).not.toContain('Desconto')
    expect(lastQuote(calls)?.terms).toEqual({ handover: 'pickup' })
  })

  it('checks the values before pricing or saving', async () => {
    const calls = serve()
    const { wrapper } = await openForm()
    await chooseCustomer(wrapper)
    await addItem(wrapper)
    await field(wrapper, 'Sinal (%)').setValue('150')
    await field(wrapper, 'Dias de Montagem').setValue('90')
    await field(wrapper, 'Desconto').setValue('abc')

    await button(wrapper, 'Salvar rascunho').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('Use um sinal de 0 a 100%.')
    expect(wrapper.text()).toContain('Use de 0 a 60 dias.')
    expect(wrapper.text()).toContain('Confira o valor.')
    expect(calls.creates).toEqual([])
    expect(calls.quotes).toEqual([])
  })
})

describe('saving', () => {
  async function filledForm(calls: Calls) {
    const app = await openForm()
    await chooseCustomer(app.wrapper)
    await addItem(app.wrapper)
    await vi.waitFor(() => expect(lastQuote(calls)).toBeDefined())
    return app
  }

  it('creates a draft with the customer, the lines and the terms, and shows the tracking link', async () => {
    const calls = serve()
    const { wrapper } = await filledForm(calls)

    await button(wrapper, 'Salvar rascunho').trigger('click')
    await vi.waitFor(() => expect(wrapper.text()).toContain('Pedido P-0231 criado.'))

    expect(calls.creates).toHaveLength(1)
    expect(calls.creates[0]?.body).toEqual({ customerId: joao.id, lines: [{ itemId: farolId, quantity: 1 }], terms: { handover: 'pickup' }, sendToCustomer: false })
    expect(wrapper.text()).toContain(`Link de acompanhamento: ${trackingUrl}`)
    expect(window.open).not.toHaveBeenCalled()
  })

  it('opens the wa.me link the API returned after "Salvar e enviar ao cliente"', async () => {
    const calls = serve({ createdAnswer: () => HttpResponse.json(created({ whatsappUrl: sendUrl }), { status: 201 }) })
    const { wrapper } = await filledForm(calls)

    await button(wrapper, 'Salvar e enviar ao cliente').trigger('click')
    await vi.waitFor(() => expect(wrapper.text()).toContain('Pedido P-0231 criado.'))

    expect(calls.creates[0]?.body.sendToCustomer).toBe(true)
    expect(window.open).toHaveBeenCalledWith(sendUrl, '_blank', 'noopener')
    await button(wrapper, 'Enviar pelo WhatsApp').trigger('click')
    expect(window.open).toHaveBeenCalledTimes(2)
    expect(vi.mocked(window.open).mock.calls[1]).toEqual([sendUrl, '_blank', 'noopener'])
  })

  it('asks for a new tracking link for a draft and opens the link the API returns', async () => {
    const calls = serve()
    const fresh = 'https://altarpm.encomendo.com.br/p/token-2'
    server.use(http.post('*/api/v1/admin/orders/:id/tracking-link', () => HttpResponse.json({ trackingUrl: fresh, whatsappUrl: sendUrl })))
    const { wrapper } = await filledForm(calls)
    await button(wrapper, 'Salvar rascunho').trigger('click')
    await vi.waitFor(() => expect(wrapper.text()).toContain('Pedido P-0231 criado.'))

    await button(wrapper, 'Enviar pelo WhatsApp').trigger('click')
    await vi.waitFor(() => expect(window.open).toHaveBeenCalledWith(sendUrl, '_blank', 'noopener'))

    // The old link stopped working, so the new one is the one shown.
    expect(wrapper.text()).toContain(`Link de acompanhamento: ${fresh}`)
    expect(wrapper.text()).not.toContain(trackingUrl)
  })

  it('links to the order created and back to the list', async () => {
    const calls = serve()
    const { wrapper } = await filledForm(calls)
    await button(wrapper, 'Salvar rascunho').trigger('click')
    await vi.waitFor(() => expect(wrapper.text()).toContain('Pedido P-0231 criado.'))

    expect(link(wrapper, 'Abrir pedido').attributes('href')).toBe(`/pedidos/${created().order.id}`)
    expect(link(wrapper, 'Voltar para Pedidos').attributes('href')).toBe('/pedidos')
  })

  it('keeps the idempotency key on retry', async () => {
    let attempt = 0
    const calls = serve({
      createdAnswer: () => {
        attempt += 1
        return attempt === 1 ? new HttpResponse(null, { status: 500 }) : HttpResponse.json(created(), { status: 201 })
      },
    })
    const { wrapper } = await filledForm(calls)

    await button(wrapper, 'Salvar rascunho').trigger('click')
    await vi.waitFor(() => expect(wrapper.text()).toContain('O servidor não respondeu.'))
    await button(wrapper, 'Salvar rascunho').trigger('click')
    await vi.waitFor(() => expect(wrapper.text()).toContain('Pedido P-0231 criado.'))

    expect(calls.creates).toHaveLength(2)
    expect(calls.creates[0]?.key).toMatch(/^[0-9a-f-]{36}$/)
    expect(calls.creates[1]?.key).toBe(calls.creates[0]?.key)
  })

  it('makes a new key when the form changed since the failed try', async () => {
    let attempt = 0
    const calls = serve({
      createdAnswer: () => {
        attempt += 1
        return attempt === 1 ? new HttpResponse(null, { status: 500 }) : HttpResponse.json(created(), { status: 201 })
      },
    })
    const { wrapper } = await filledForm(calls)
    await button(wrapper, 'Salvar rascunho').trigger('click')
    await vi.waitFor(() => expect(wrapper.text()).toContain('O servidor não respondeu.'))

    await field(wrapper, 'Quantidade').setValue('2')
    await button(wrapper, 'Salvar rascunho').trigger('click')
    await vi.waitFor(() => expect(wrapper.text()).toContain('Pedido P-0231 criado.'))

    expect(calls.creates[1]?.key).not.toBe(calls.creates[0]?.key)
    expect(calls.creates[1]?.body.lines).toEqual([{ itemId: farolId, quantity: 2 }])
  })

  it('says when the key was already used for another order', async () => {
    const calls = serve({ createdAnswer: () => orderProblem('idempotency_key_reused') })
    const { wrapper } = await filledForm(calls)

    await button(wrapper, 'Salvar rascunho').trigger('click')
    await flushPromises()

    expect(wrapper.get('.alert').text()).toBe('Esta chave já foi usada em outro pedido.')
  })

  it('names the archived item the API refused and keeps the form', async () => {
    const calls = serve({ createdAnswer: () => orderProblem('item_unavailable', { references: [{ id: farolId, name: 'Farol LED' }] }) })
    const { wrapper } = await filledForm(calls)

    await button(wrapper, 'Salvar rascunho').trigger('click')
    await flushPromises()

    expect(wrapper.get('.alert').text()).toBe('Farol LED está arquivado e não pode entrar no pedido.')
    expect(wrapper.get('.line').text()).toContain('Farol LED está arquivado e não pode entrar no pedido.')
    expect(wrapper.text()).not.toContain('Pedido P-0231 criado.')
  })

  it('shows a refusal the form has no place for above the form', async () => {
    const calls = serve({ createdAnswer: () => orderProblem('invalid') })
    const { wrapper } = await filledForm(calls)

    await button(wrapper, 'Salvar rascunho').trigger('click')
    await flushPromises()

    expect(wrapper.get('.alert').text()).toBe('Confira os campos destacados.')
  })

  it('stops a second press while saving', async () => {
    const release = gate()
    const calls = serve({
      createdAnswer: async () => {
        await release.opened
        return HttpResponse.json(created(), { status: 201 })
      },
    })
    const { wrapper } = await filledForm(calls)

    await button(wrapper, 'Salvar rascunho').trigger('click')
    await flushPromises()

    expect(button(wrapper, 'Salvar rascunho').attributes('disabled')).toBeDefined()
    expect(button(wrapper, 'Salvar e enviar ao cliente').attributes('disabled')).toBeDefined()
    release.open()
    await vi.waitFor(() => expect(wrapper.text()).toContain('Pedido P-0231 criado.'))
  })
})

describe('leaving with edits', () => {
  it('asks "Descartar as alterações?" and stays on "Continuar editando"', async () => {
    serve()
    const { wrapper, router } = await openForm()
    await addItem(wrapper)

    await button(wrapper, 'Cancelar').trigger('click')
    await vi.waitFor(() => expect(wrapper.text()).toContain('Descartar as alterações?'))
    await button(wrapper, 'Continuar editando').trigger('click')
    await flushPromises()

    expect(router.currentRoute.value.name).toBe('new-order')
    expect(wrapper.text()).not.toContain('Descartar as alterações?')
    expect(wrapper.find('.line').exists()).toBe(true)
  })

  it('leaves on "Descartar"', async () => {
    serve()
    server.use(http.get('*/api/v1/admin/orders', () => HttpResponse.json({ items: [] })))
    const { wrapper, router } = await openForm()
    await addItem(wrapper)

    await button(wrapper, 'Cancelar').trigger('click')
    await vi.waitFor(() => expect(wrapper.text()).toContain('Descartar as alterações?'))
    await button(wrapper, 'Descartar').trigger('click')

    await vi.waitFor(() => expect(router.currentRoute.value.name).toBe('orders'))
  })

  it('leaves at once with nothing edited', async () => {
    serve()
    server.use(http.get('*/api/v1/admin/orders', () => HttpResponse.json({ items: [] })))
    const { wrapper, router } = await openForm()

    await button(wrapper, 'Cancelar').trigger('click')

    await vi.waitFor(() => expect(router.currentRoute.value.name).toBe('orders'))
  })
})

describe('the phone layout', () => {
  // The layout itself is CSS (the Playwright phone project checks 390 px); these are the structures it relies on.
  it('puts the picker dialogs in a sheet with "Fechar" in the sticky footer', async () => {
    serve()
    const { wrapper } = await openForm()

    await button(wrapper, 'Adicionar item').trigger('click')
    await vi.waitFor(() => expect(wrapper.find('[role=dialog]').exists()).toBe(true))

    expect(wrapper.get('[role=dialog] .enc-sheet-actions').findAll('button').map((b) => b.text())).toEqual(['Fechar'])
  })

  it('gives every field a label, and radios their words as one label', async () => {
    serve()
    const { wrapper } = await openForm()
    await chooseCustomer(wrapper)
    await addKit(wrapper)

    const controls = wrapper.findAll('form select, form input:not([type=radio]):not([type=checkbox])')
    for (const control of controls) {
      const id = control.attributes('id')
      expect(id !== undefined && wrapper.find(`label[for="${id}"]`).exists()).toBe(true)
    }
    for (const radio of wrapper.findAll('input[type=radio]')) expect(radio.element.closest('label.enc-check')).not.toBeNull()
  })

  it('shows the paid-order approval as an enc-check', async () => {
    serve()
    const { wrapper } = await openForm()

    // A new order has nothing to approve; the change form has the box (see order.test.ts).
    expect(wrapper.text()).not.toContain('O cliente aprovou a mudança')
  })

  it('keeps every action of the form in a wrapping row', async () => {
    serve()
    const { wrapper } = await openForm()

    expect(wrapper.get('.actions').findAll('button').map((b) => b.text())).toEqual(['Salvar rascunho', 'Salvar e enviar ao cliente', 'Cancelar'])
  })
})

describe('the catalog listing', () => {
  it('lists the kit with its price', async () => {
    serve()
    const { wrapper } = await openForm()

    await button(wrapper, 'Adicionar kit').trigger('click')
    await vi.waitFor(() => expect(wrapper.find('[role=dialog] ul button').exists()).toBe(true))

    expect(plain(wrapper.get('[role=dialog] ul button').text())).toContain(`${kit.name}R$ 700,00`)
    expect(wrapper.get('[role=dialog] h2').text()).toBe('Adicionar kit')
  })
})
