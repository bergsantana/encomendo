import type { Customer, CustomerSummary, OrderSummary, Permission, TimeZone } from '@encomendo/api'
import { enableAutoUnmount, flushPromises, type VueWrapper } from '@vue/test-utils'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'

import { loadPage } from '../../pageLoad'
import { button, closeApps, field, link, openApp } from '../../testing/app'
import { authSession, gate } from '../../testing/fixtures'
import { todayHandler } from '../../testing/today'
import { encomendaMontagem, joao, maria, orderProblem, orderSummary, prontaEntrega, settingsUsing, summaryOf } from '../../testing/orders'

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

const writer: Permission[] = ['order:read', 'order:write', 'customer:write']

const signedInAs = (role: 'owner' | 'admin' | 'staff' | 'viewer', permissions: Permission[]) =>
  server.use(http.post('*/auth/refresh', () => HttpResponse.json(authSession({ role, permissions }))))

const joaoRow = summaryOf(joao, { orders: 2, lastOrderAt: '2026-11-25T15:00:00Z' })
const mariaRow = summaryOf(maria)

// P-0198 delivers earlier, so the API lists it first; the drawer shows the newest order (the highest number) first.
const earlier: OrderSummary = orderSummary(prontaEntrega, 'separado', { id: '0190f3c4-5e1a-7b2c-8d3e-000000000202', number: 'P-0198', deliveryDate: '2026-11-20' })
const later: OrderSummary = orderSummary(encomendaMontagem, 'em_montagem', { number: 'P-0231', deliveryDate: '2026-12-12' })

interface Backend {
  rows?: CustomerSummary[]
  customer?: Customer
  orders?: OrderSummary[]
  timeZone?: TimeZone
}

/** serve answers the reads of the screen; `seen` records the list and order queries. */
function serve(backend: Backend = {}) {
  const seen = { lists: [] as URL[], orders: [] as URL[] }
  server.use(
    todayHandler(),
    http.get('*/api/v1/admin/settings', () => HttpResponse.json(settingsUsing(encomendaMontagem, { timeZone: backend.timeZone ?? 'America/Sao_Paulo' }))),
    http.get('*/api/v1/admin/customers', ({ request }) => {
      const url = new URL(request.url)
      seen.lists.push(url)
      return HttpResponse.json({ items: backend.rows ?? [joaoRow, mariaRow] })
    }),
    http.get('*/api/v1/admin/customers/:id', ({ params }) => {
      const customer = backend.customer ?? joao
      return params.id === customer.id ? HttpResponse.json(customer) : orderProblem('not_found')
    }),
    http.get('*/api/v1/admin/orders', ({ request }) => {
      seen.orders.push(new URL(request.url))
      return HttpResponse.json({ items: backend.orders ?? [earlier, later] })
    }),
  )
  return seen
}

async function openClientes(path = '/clientes', name = 'João Pereira') {
  const app = await openApp(path)
  await vi.waitFor(() => expect(app.wrapper.text()).toContain(name))
  return app
}

/** openCustomer opens the drawer from the customer's name in the table. */
async function openCustomer(wrapper: VueWrapper, name = 'João Pereira') {
  await link(wrapper, name).trigger('click')
  await vi.waitFor(() => expect(wrapper.find('[role=dialog]').exists()).toBe(true))
  await flushPromises()
  return wrapper.get('[role=dialog]')
}

const plain = (text: string) => text.replace(/\u00a0/g, ' ')

beforeEach(() => {
  signedInAs('owner', writer)
  serve()
})

describe('the Clientes menu entry and route', () => {
  it('is in the menu for every role that reads orders', async () => {
    signedInAs('viewer', ['order:read'])

    const { wrapper } = await openApp('/hoje')

    expect(link(wrapper, 'Clientes').attributes('href')).toBe('/clientes')
  })

  it('is hidden from a role without order:read, and its address leads to Hoje', async () => {
    signedInAs('viewer', ['catalog:read'])

    const { wrapper, router } = await openApp('/clientes')

    expect(router.currentRoute.value.name).toBe('hoje')
    expect(wrapper.findAll('a').map((a) => a.text())).not.toContain('Clientes')
  })

  it('keeps "Clientes" marked while a customer is open', async () => {
    const { wrapper } = await openClientes(`/clientes/${joao.id}`)

    expect(link(wrapper, 'Clientes').classes()).toContain('menu-link--active')
  })
})

describe('the customer list', () => {
  it('lists customers with WhatsApp, orders and the last order', async () => {
    const { wrapper } = await openClientes()

    expect(wrapper.findAll('thead th').map((th) => th.text())).toEqual(['Nome', 'WhatsApp', 'Pedidos', 'Último pedido'])
    const rows = wrapper.findAll('tbody tr').map((row) => plain(row.text()))
    expect(rows[0]).toContain('João Pereira')
    expect(rows[0]).toContain('(92) 99123-4567')
    expect(rows[0]).toContain('2')
    expect(rows[0]).toContain('25/11/2026')
    expect(rows[1]).toContain('Maria Souza')
    expect(rows[1]).toContain('—')
  })

  it("shows the day of the last order in the store's time zone", async () => {
    // 03:30 UTC is still the 25th in Manaus (UTC-4) and already the 26th in Brasília (UTC-3).
    const late = summaryOf(joao, { orders: 1, lastOrderAt: '2026-11-26T03:30:00Z' })
    serve({ rows: [late], timeZone: 'America/Manaus' })
    const manaus = await openClientes()
    expect(plain(manaus.wrapper.get('tbody tr').text())).toContain('25/11/2026')

    serve({ rows: [late], timeZone: 'America/Sao_Paulo' })
    const brasilia = await openClientes()
    expect(plain(brasilia.wrapper.get('tbody tr').text())).toContain('26/11/2026')
  })

  it('still lists customers when the settings cannot be read', async () => {
    server.use(http.get('*/api/v1/admin/settings', () => new HttpResponse(null, { status: 500 })))

    const { wrapper } = await openClientes()

    expect(wrapper.get('tbody tr').text()).toContain('João Pereira')
    expect(wrapper.find('[role=alert]').exists()).toBe(false)
  })

  it('says it is loading, then that there are no customers yet', async () => {
    const release = gate()
    server.use(
      http.get('*/api/v1/admin/customers', async () => {
        await release.opened
        return HttpResponse.json({ items: [] })
      }),
    )

    const { wrapper } = await openApp('/clientes')
    expect(wrapper.text()).toContain('Carregando os clientes…')
    release.open()
    await vi.waitFor(() => expect(wrapper.text()).toContain('Nenhum cliente ainda.'))
  })

  it('reads the next page on "Carregar mais"', async () => {
    const cursors: (string | null)[] = []
    server.use(
      http.get('*/api/v1/admin/customers', ({ request }) => {
        const cursor = new URL(request.url).searchParams.get('cursor')
        cursors.push(cursor)
        return cursor === null ? HttpResponse.json({ items: [joaoRow], nextCursor: 'p2' }) : HttpResponse.json({ items: [mariaRow] })
      }),
    )
    const { wrapper } = await openClientes()

    await button(wrapper, 'Carregar mais').trigger('click')
    await vi.waitFor(() => expect(wrapper.text()).toContain('Maria Souza'))

    expect(cursors).toEqual([null, 'p2'])
    expect(wrapper.text()).toContain('João Pereira')
    expect(wrapper.findAll('button').map((b) => b.text())).not.toContain('Carregar mais')
  })

  it('offers "Tentar de novo" when the list cannot be read', async () => {
    server.use(http.get('*/api/v1/admin/customers', () => new HttpResponse(null, { status: 500 })))

    const { wrapper } = await openApp('/clientes')
    await vi.waitFor(() => expect(wrapper.text()).toContain('O servidor não respondeu.'))

    expect(button(wrapper, 'Tentar de novo').exists()).toBe(true)
  })
})

describe('the search', () => {
  it('asks for the name typed, on Enter', async () => {
    const seen = serve()
    const { wrapper } = await openClientes()

    await field(wrapper, 'Buscar por nome ou WhatsApp').setValue('mar')
    await wrapper.get('form[role=search]').trigger('submit')
    await flushPromises()

    expect(seen.lists.at(-1)?.searchParams.get('q')).toBe('mar')
  })

  it('asks for the digits of a number typed the way people write it', async () => {
    const seen = serve()
    const { wrapper } = await openClientes()

    await field(wrapper, 'Buscar por nome ou WhatsApp').setValue('(92) 99123')
    await wrapper.get('form[role=search]').trigger('submit')
    await flushPromises()

    expect(seen.lists.at(-1)?.searchParams.get('q')).toBe('9299123')
  })

  it('waits a moment after typing, then reads the list again', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    try {
      const seen = serve()
      const { wrapper } = await openClientes()
      const reads = seen.lists.length

      await field(wrapper, 'Buscar por nome ou WhatsApp').setValue('joão')
      expect(seen.lists).toHaveLength(reads)
      await vi.advanceTimersByTimeAsync(400)
      await flushPromises()

      expect(seen.lists).toHaveLength(reads + 1)
      expect(seen.lists.at(-1)?.searchParams.get('q')).toBe('joão')
    } finally {
      vi.useRealTimers()
    }
  })

  it('says "Nenhum cliente encontrado." when nothing matches', async () => {
    const { wrapper } = await openClientes()
    server.use(http.get('*/api/v1/admin/customers', () => HttpResponse.json({ items: [] })))

    await field(wrapper, 'Buscar por nome ou WhatsApp').setValue('zzz')
    await wrapper.get('form[role=search]').trigger('submit')
    await flushPromises()

    expect(wrapper.text()).toContain('Nenhum cliente encontrado.')
    expect(wrapper.text()).not.toContain('Nenhum cliente ainda.')
  })
})

describe('a new customer', () => {
  /** drawer opens "Novo cliente" and fills the main fields. */
  async function fillNew(wrapper: VueWrapper) {
    await button(wrapper, 'Novo cliente').trigger('click')
    await flushPromises()
    await field(wrapper, 'Nome').setValue('Carla Dias')
    await field(wrapper, 'WhatsApp').setValue('(92) 98111-2222')
  }

  it('is offered to roles that write customers, and opens an empty form', async () => {
    const { wrapper } = await openClientes()

    await button(wrapper, 'Novo cliente').trigger('click')
    await flushPromises()

    expect(wrapper.get('[role=dialog]').text()).toContain('Novo cliente')
    expect((field(wrapper, 'Nome').element as HTMLInputElement).value).toBe('')
    expect(wrapper.text()).toContain('Com DDD, como (92) 99123-4567.')
  })

  it('saves with the number as digits and no addresses, says so, and lists again', async () => {
    let body: unknown
    server.use(http.post('*/api/v1/admin/customers', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ ...maria, name: 'Carla Dias' }, { status: 201 })
    }))
    const seen = serve()
    const { wrapper } = await openClientes()
    const reads = seen.lists.length
    await fillNew(wrapper)

    await wrapper.get('[role=dialog] form').trigger('submit')
    await flushPromises()

    expect(body).toEqual({ name: 'Carla Dias', phone: '5592981112222', email: null, notes: '', addresses: [] })
    expect(wrapper.find('[role=dialog]').exists()).toBe(false)
    expect(wrapper.get('[role=status]').text()).toBe('Cliente salvo.')
    await vi.waitFor(() => expect(seen.lists.length).toBe(reads + 1))
  })

  it('asks for the name and the WhatsApp before calling the API', async () => {
    let called = false
    server.use(http.post('*/api/v1/admin/customers', () => {
      called = true
      return HttpResponse.json(joao, { status: 201 })
    }))
    const { wrapper } = await openClientes()
    await button(wrapper, 'Novo cliente').trigger('click')
    await flushPromises()

    await wrapper.get('[role=dialog] form').trigger('submit')
    await flushPromises()

    expect(called).toBe(false)
    const text = wrapper.get('[role=dialog]').text()
    expect(text).toContain('Informe o nome do cliente.')
    expect(text).toContain('Informe o WhatsApp do cliente.')
    expect(text).toContain('Confira os campos destacados.')
  })

  it('checks the number, the e-mail and the lengths', async () => {
    const { wrapper } = await openClientes()
    await button(wrapper, 'Novo cliente').trigger('click')
    await flushPromises()
    await field(wrapper, 'Nome').setValue('x'.repeat(121))
    await field(wrapper, 'WhatsApp').setValue('1234')
    await field(wrapper, 'E-mail').setValue('sem-arroba')
    await field(wrapper, 'Observações').setValue('y'.repeat(2001))

    await wrapper.get('[role=dialog] form').trigger('submit')
    await flushPromises()

    const text = wrapper.get('[role=dialog]').text()
    expect(text).toContain('Use um nome de até 120 caracteres.')
    expect(text).toContain('Confira o número: use DDD e 8 ou 9 dígitos.')
    expect(text).toContain('Confira o e-mail.')
    expect(text).toContain('Use observações de até 2000 caracteres.')
  })

  it('says the WhatsApp is taken, next to the field', async () => {
    server.use(http.post('*/api/v1/admin/customers', () => orderProblem('phone_taken')))
    const { wrapper } = await openClientes()
    await fillNew(wrapper)

    await wrapper.get('[role=dialog] form').trigger('submit')
    await flushPromises()

    const dialog = wrapper.get('[role=dialog]')
    expect(dialog.text()).toContain('Já existe um cliente com este WhatsApp.')
    expect(dialog.get('input[aria-invalid=true]').attributes('inputmode')).toBe('tel')
    expect(wrapper.find('[role=dialog]').exists()).toBe(true)
  })

  it('shows what the API refuses on a field', async () => {
    server.use(
      http.post('*/api/v1/admin/customers', () =>
        HttpResponse.json(
          { status: 400, code: 'invalid', title: 'Dados inválidos', errors: [{ field: 'email', message: 'Confira o e-mail.' }] },
          { status: 400, headers: { 'Content-Type': 'application/problem+json' } },
        ),
      ),
    )
    const { wrapper } = await openClientes()
    await fillNew(wrapper)

    await wrapper.get('[role=dialog] form').trigger('submit')
    await flushPromises()

    expect(wrapper.get('[role=dialog]').text()).toContain('Confira o e-mail.')
  })

  it('closes without saving on "Cancelar"', async () => {
    const { wrapper } = await openClientes()
    await fillNew(wrapper)

    await button(wrapper.get('[role=dialog]'), 'Cancelar').trigger('click')

    expect(wrapper.find('[role=dialog]').exists()).toBe(false)
    expect(wrapper.find('[role=status]').exists()).toBe(false)
  })
})

describe('editing a customer', () => {
  it('opens the customer at its own address, filled in, with its addresses', async () => {
    const { wrapper, router } = await openClientes()

    const dialog = await openCustomer(wrapper)

    expect(router.currentRoute.value.fullPath).toBe(`/clientes/${joao.id}`)
    expect(dialog.get('h2').text()).toBe('Cliente')
    expect((field(dialog, 'Nome').element as HTMLInputElement).value).toBe('João Pereira')
    expect((field(dialog, 'WhatsApp').element as HTMLInputElement).value).toBe('(92) 99123-4567')
    expect((field(dialog, 'E-mail').element as HTMLInputElement).value).toBe('joao@example.com')
    expect((field(dialog, 'Observações').element as HTMLTextAreaElement).value).toBe('Prefere retirar na loja.')
    expect((field(dialog, 'Nome do endereço').element as HTMLInputElement).value).toBe('Casa')
    expect((field(dialog, 'Rua').element as HTMLInputElement).value).toBe('Rua das Palmeiras')
    expect((field(dialog, 'UF').element as HTMLSelectElement).value).toBe('AM')
  })

  it('opens from a pasted address, and a reload lands on the same customer', async () => {
    const { wrapper } = await openClientes(`/clientes/${joao.id}`)

    await vi.waitFor(() => expect(wrapper.find('[role=dialog]').exists()).toBe(true))
    await flushPromises()

    expect((field(wrapper.get('[role=dialog]'), 'Nome').element as HTMLInputElement).value).toBe('João Pereira')
  })

  it('says the customer is not there and shows the list when the address is wrong', async () => {
    const { wrapper, router } = await openApp('/clientes/0190f3c4-5e1a-7b2c-8d3e-00000000dead')
    await vi.waitFor(() => expect(wrapper.text()).toContain('Cliente não encontrado.'))
    await flushPromises()

    expect(router.currentRoute.value.name).toBe('customers')
    expect(wrapper.find('[role=dialog]').exists()).toBe(false)
  })

  it('saves every field and address with the version read, keeping the address id', async () => {
    let body: unknown
    server.use(http.put(`*/api/v1/admin/customers/${joao.id}`, async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ ...joao, version: 4 })
    }))
    const { wrapper, router } = await openClientes()
    const dialog = await openCustomer(wrapper)
    await field(dialog, 'Observações').setValue('Prefere entrega.')
    await field(dialog, 'CEP').setValue('69057-000')

    await dialog.get('form').trigger('submit')
    await flushPromises()

    expect(body).toEqual({
      name: 'João Pereira',
      phone: '5592991234567',
      email: 'joao@example.com',
      notes: 'Prefere entrega.',
      addresses: [
        {
          id: joao.addresses[0]?.id,
          label: 'Casa',
          cep: '69057000',
          street: 'Rua das Palmeiras',
          number: '120',
          complement: 'Apto 4',
          district: 'Adrianópolis',
          city: 'Manaus',
          state: 'AM',
        },
      ],
      version: 3,
    })
    expect(wrapper.get('[role=status]').text()).toBe('Cliente salvo.')
    await vi.waitFor(() => expect(router.currentRoute.value.name).toBe('customers'))
    expect(wrapper.find('[role=dialog]').exists()).toBe(false)
  })

  it('adds an address, with its own fields, and sends it without an id', async () => {
    let body: { addresses: Record<string, unknown>[] } | undefined
    server.use(http.put(`*/api/v1/admin/customers/${joao.id}`, async ({ request }) => {
      body = (await request.json()) as { addresses: Record<string, unknown>[] }
      return HttpResponse.json(joao)
    }))
    const { wrapper } = await openClientes()
    const dialog = await openCustomer(wrapper)

    await button(dialog, 'Adicionar endereço').trigger('click')
    const second = dialog.findAll('fieldset').filter((group) => group.find('legend')?.text() === 'Endereço 2')[0]
    if (second === undefined) throw new Error('no second address')
    await field(second, 'Nome do endereço').setValue('Trabalho')
    await field(second, 'CEP').setValue('69010-000')
    await field(second, 'Rua').setValue('Av. Eduardo Ribeiro')
    await field(second, 'Cidade').setValue('Manaus')
    await field(second, 'UF').setValue('AM')
    await dialog.get('form').trigger('submit')
    await flushPromises()

    expect(body?.addresses).toHaveLength(2)
    expect(body?.addresses[1]).toEqual({
      label: 'Trabalho',
      cep: '69010000',
      street: 'Av. Eduardo Ribeiro',
      number: '',
      complement: '',
      district: '',
      city: 'Manaus',
      state: 'AM',
    })
  })

  it('asks for what an address needs, next to its own fields', async () => {
    const { wrapper } = await openClientes()
    const dialog = await openCustomer(wrapper)
    await button(dialog, 'Adicionar endereço').trigger('click')

    await dialog.get('form').trigger('submit')
    await flushPromises()

    const second = dialog.findAll('fieldset').filter((group) => group.find('legend')?.text() === 'Endereço 2')[0]
    const text = second?.text() ?? ''
    expect(text).toContain('Informe o nome do endereço.')
    expect(text).toContain('Confira o CEP: use 8 dígitos.')
    expect(text).toContain('Informe a rua.')
    expect(text).toContain('Informe a cidade.')
    expect(text).toContain('Escolha a UF.')
    expect(dialog.findAll('fieldset').find((group) => group.find('legend')?.text() === 'Casa')?.text()).not.toContain('Informe')
  })

  it('checks the length of an address field before calling the API', async () => {
    let called = false
    server.use(http.put(`*/api/v1/admin/customers/${joao.id}`, () => {
      called = true
      return HttpResponse.json(joao)
    }))
    const { wrapper } = await openClientes()
    const dialog = await openCustomer(wrapper)
    await field(dialog, 'Nome do endereço').setValue('x'.repeat(41))
    await field(dialog, 'Rua').setValue('y'.repeat(201))
    await field(dialog, 'Cidade').setValue('z'.repeat(101))

    await dialog.get('form').trigger('submit')
    await flushPromises()

    expect(called).toBe(false)
    const text = dialog.text()
    expect(text).toContain('Use um nome de endereço de até 40 caracteres.')
    expect(text).toContain('Use uma rua de até 200 caracteres.')
    expect(text).toContain('Use uma cidade de até 100 caracteres.')
  })

  it('does not bring the drawer back when the screen is left open and visited again', async () => {
    const { wrapper, router } = await openClientes()
    await openCustomer(wrapper)

    await router.push({ name: 'hoje' })
    await flushPromises()
    await router.push({ name: 'customers' })
    await vi.waitFor(() => expect(wrapper.text()).toContain('João Pereira'))
    await flushPromises()

    expect(wrapper.find('[role=dialog]').exists()).toBe(false)
  })

  it('shows an address error the API names by position', async () => {
    server.use(
      http.put(`*/api/v1/admin/customers/${joao.id}`, () =>
        HttpResponse.json(
          { status: 400, code: 'invalid', title: 'Dados inválidos', errors: [{ field: 'addresses[0].number', message: 'Use um número de até 20 caracteres.' }] },
          { status: 400, headers: { 'Content-Type': 'application/problem+json' } },
        ),
      ),
    )
    const { wrapper } = await openClientes()
    const dialog = await openCustomer(wrapper)

    await dialog.get('form').trigger('submit')
    await flushPromises()

    expect(dialog.findAll('fieldset').find((group) => group.find('legend')?.text() === 'Casa')?.text()).toContain('Use um número de até 20 caracteres.')
  })

  it('removes an address, and sends the customer without it', async () => {
    let body: { addresses: unknown[] } | undefined
    server.use(http.put(`*/api/v1/admin/customers/${joao.id}`, async ({ request }) => {
      body = (await request.json()) as { addresses: unknown[] }
      return HttpResponse.json({ ...joao, addresses: [] })
    }))
    const { wrapper } = await openClientes()
    const dialog = await openCustomer(wrapper)

    await button(dialog, 'Remover endereço').trigger('click')
    expect(dialog.text()).not.toContain('Rua das Palmeiras')
    await dialog.get('form').trigger('submit')
    await flushPromises()

    expect(body?.addresses).toEqual([])
  })

  it('stops adding addresses at ten', async () => {
    const { wrapper } = await openClientes()
    await button(wrapper, 'Novo cliente').trigger('click')
    await flushPromises()
    const dialog = wrapper.get('[role=dialog]')

    for (let n = 0; n < 10; n += 1) await button(dialog, 'Adicionar endereço').trigger('click')

    expect(dialog.findAll('fieldset fieldset')).toHaveLength(10)
    expect(button(dialog, 'Adicionar endereço').attributes('disabled')).toBeDefined()
  })

  it('says someone saved first, and "Recarregar" brings the version they saved', async () => {
    let reads = 0
    server.use(
      http.get(`*/api/v1/admin/customers/${joao.id}`, () => {
        reads += 1
        return HttpResponse.json({ ...joao, name: reads === 1 ? 'João Pereira' : 'João P. Filho', version: reads === 1 ? 3 : 4 })
      }),
      http.put(`*/api/v1/admin/customers/${joao.id}`, () => orderProblem('version_conflict')),
    )
    const { wrapper } = await openClientes()
    const dialog = await openCustomer(wrapper)

    await dialog.get('form').trigger('submit')
    await flushPromises()
    expect(dialog.get('[role=alert]').text()).toContain('Outra pessoa alterou este cliente. Recarregue para ver a versão atual.')

    await button(dialog, 'Recarregar').trigger('click')
    await flushPromises()

    expect((field(wrapper.get('[role=dialog]'), 'Nome').element as HTMLInputElement).value).toBe('João P. Filho')
    expect(wrapper.find('[role=dialog] [role=alert]').exists()).toBe(false)
  })
})

describe("the customer's orders", () => {
  it("asks for this customer's orders of every kind, and lists them newest first with number, status and date", async () => {
    const seen = serve()
    const { wrapper } = await openClientes()

    const dialog = await openCustomer(wrapper)

    const query = seen.orders.at(-1)?.searchParams
    expect(query?.get('customerId')).toBe(joao.id)
    expect(query?.get('scope')).toBe('all')
    const section = dialog.get('section[aria-labelledby=customer-orders]')
    expect(section.get('h2').text()).toBe('Pedidos deste cliente')
    expect(section.findAll('li').map((li) => plain(li.text()))).toEqual([
      'P-0231Em montagemEntrega prevista: 12/12/2026',
      'P-0198SeparadoEntrega prevista: 20/11/2026',
    ])
  })

  it("takes each order's words and color from its own workflow", async () => {
    const { wrapper } = await openClientes()

    const dialog = await openCustomer(wrapper)

    const badges = dialog.findAll('[data-tone]')
    expect(badges.map((badge) => [badge.text(), badge.attributes('data-tone')])).toEqual([
      ['Em montagem', 'assembly'],
      ['Separado', 'ready'],
    ])
  })

  it('sorts by order number, not by the order the API sent', async () => {
    const middle = orderSummary(encomendaMontagem, 'confirmado', { id: '0190f3c4-5e1a-7b2c-8d3e-000000000203', number: 'P-0210', deliveryDate: '2026-12-01' })
    serve({ orders: [earlier, middle, later] })
    const { wrapper } = await openClientes()

    const dialog = await openCustomer(wrapper)

    expect(dialog.findAll('section li .number').map((n) => n.text())).toEqual(['P-0231', 'P-0210', 'P-0198'])
  })

  it('stops at 500 orders and says the list is the nearest deliveries', async () => {
    const cursors: (string | null)[] = []
    server.use(
      http.get('*/api/v1/admin/orders', ({ request }) => {
        const cursor = new URL(request.url).searchParams.get('cursor')
        cursors.push(cursor)
        // Every page says there is another one.
        return HttpResponse.json({ items: [orderSummary(encomendaMontagem, 'confirmado', { id: `0190f3c4-5e1a-7b2c-8d3e-0000000009${cursors.length}`, number: `P-${String(cursors.length).padStart(4, '0')}` })], nextCursor: `o${cursors.length + 1}` })
      }),
    )
    const { wrapper } = await openClientes()

    const dialog = await openCustomer(wrapper)

    expect(cursors).toEqual([null, 'o2', 'o3', 'o4', 'o5'])
    expect(dialog.text()).toContain('Mostrando os 500 pedidos com entrega mais próxima.')
  })

  it('does not say it when every order was read', async () => {
    const { wrapper } = await openClientes()

    const dialog = await openCustomer(wrapper)

    expect(dialog.text()).not.toContain('Mostrando os 500 pedidos')
  })

  it('reads every page of the orders', async () => {
    const cursors: (string | null)[] = []
    server.use(
      http.get('*/api/v1/admin/orders', ({ request }) => {
        const cursor = new URL(request.url).searchParams.get('cursor')
        cursors.push(cursor)
        return cursor === null ? HttpResponse.json({ items: [earlier], nextCursor: 'o2' }) : HttpResponse.json({ items: [later] })
      }),
    )
    const { wrapper } = await openClientes()

    const dialog = await openCustomer(wrapper)

    expect(cursors).toEqual([null, 'o2'])
    expect(dialog.findAll('section li')).toHaveLength(2)
  })

  it('says there are none, and says when they cannot be read', async () => {
    serve({ orders: [] })
    const none = await openClientes()
    expect((await openCustomer(none.wrapper)).text()).toContain('Nenhum pedido ainda.')

    server.use(http.get('*/api/v1/admin/orders', () => new HttpResponse(null, { status: 500 })))
    const failed = await openClientes()
    const dialog = await openCustomer(failed.wrapper)
    expect(dialog.get('section [role=alert]').text()).toContain('O servidor não respondeu.')
  })

  it('shows no orders for a new customer', async () => {
    const { wrapper } = await openClientes()

    await button(wrapper, 'Novo cliente').trigger('click')
    await flushPromises()

    expect(wrapper.get('[role=dialog]').text()).not.toContain('Pedidos deste cliente')
  })
})

describe.each([
  ['staff', ['order:read', 'order:write']],
  ['viewer', ['order:read']],
] as const)('a %s, who cannot write customers', (role, permissions) => {
  beforeEach(() => signedInAs(role, [...permissions]))

  it('reads the list without "Novo cliente"', async () => {
    const { wrapper } = await openClientes()

    expect(wrapper.findAll('button').map((b) => b.text())).not.toContain('Novo cliente')
  })

  it('reads the customer as labelled lines, with its addresses and orders, and "Fechar"', async () => {
    const { wrapper } = await openClientes()

    const dialog = await openCustomer(wrapper)

    expect(dialog.get('h2').text()).toBe('Cliente')
    expect(dialog.findAll('input, textarea, select')).toHaveLength(0)
    const lines = dialog.findAll('dt').map((term) => [term.text(), dialog.findAll('dt').indexOf(term)])
    expect(lines.map(([term]) => term)).toEqual(['Nome', 'WhatsApp', 'E-mail', 'Observações', 'Endereços'])
    const text = plain(dialog.text())
    expect(text).toContain('João Pereira')
    expect(text).toContain('(92) 99123-4567')
    expect(text).toContain('joao@example.com')
    expect(text).toContain('Prefere retirar na loja.')
    expect(text).toContain('Casa: Rua das Palmeiras, 120 · Apto 4 · Adrianópolis · Manaus - AM · CEP 69057000')
    expect(text).toContain('P-0231')
    expect(button(dialog, 'Fechar').exists()).toBe(true)
    expect(dialog.findAll('button').map((b) => b.text())).not.toContain('Salvar')
  })

  it('closes with "Fechar" and never calls the API to save', async () => {
    let saved = false
    server.use(http.put(`*/api/v1/admin/customers/${joao.id}`, () => {
      saved = true
      return HttpResponse.json(joao)
    }))
    const { wrapper, router } = await openClientes()
    const dialog = await openCustomer(wrapper)

    await button(dialog, 'Fechar').trigger('click')
    await flushPromises()

    expect(wrapper.find('[role=dialog]').exists()).toBe(false)
    expect(router.currentRoute.value.name).toBe('customers')
    expect(saved).toBe(false)
  })
})

describe('the phone layout', () => {
  // The layout itself is CSS (the Playwright phone project checks 390 px); these are the structures it relies on.
  it('turns the list into cards: labelled cells, the name as the card title', async () => {
    const { wrapper } = await openClientes()

    expect(wrapper.get('table').classes()).toContain('enc-cards')
    expect(wrapper.findAll('tbody tr')[0]?.findAll('td').map((cell) => cell.attributes('data-label'))).toEqual(['WhatsApp', 'Pedidos', 'Último pedido'])
    expect(wrapper.findAll('tbody tr')[0]?.find('th a').text()).toBe('João Pereira')
  })

  it('puts "Salvar" and "Cancelar" in the sheet footer, as direct children of the form', async () => {
    const { wrapper } = await openClientes()
    const dialog = await openCustomer(wrapper)

    const footer = dialog.get('form > .enc-sheet-actions')
    expect(footer.findAll('button').map((b) => b.text())).toEqual(['Salvar', 'Cancelar'])
  })

  it('puts "Fechar" in the sheet footer of the read-only drawer', async () => {
    signedInAs('viewer', ['order:read'])
    const { wrapper } = await openClientes()
    const dialog = await openCustomer(wrapper)

    expect(dialog.get('.enc-sheet-actions').findAll('button').map((b) => b.text())).toEqual(['Fechar'])
  })

  it('gives every field of an address a label, one under the other', async () => {
    const { wrapper } = await openClientes()
    const dialog = await openCustomer(wrapper)

    const address = dialog.get('fieldset fieldset')
    const labels = address.findAll('label').map((label) => label.text())
    expect(labels).toEqual(['Nome do endereço', 'CEP', 'Rua', 'Número', 'Complemento', 'Bairro', 'Cidade', 'UF'])
    for (const control of address.findAll('input, select')) expect(address.find(`label[for="${control.attributes('id')}"]`).exists()).toBe(true)
  })
})
