import type { Attribute, Category, ItemSummary, Permission, Photo, Supplier, TenantSettings } from '@encomendo/api'
import { enableAutoUnmount, flushPromises, type VueWrapper } from '@vue/test-utils'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'

import { loadPage } from '../../pageLoad'
import { closeApps, link, openApp } from '../../testing/app'
import { authSession, gate } from '../../testing/fixtures'
import {
  bancoId,
  bancos,
  brl,
  button,
  columnHeaders,
  couros,
  field,
  group,
  guidoes,
  item,
  metais,
  ownerCosts,
  ownerItem,
  plain,
  problem,
  readOnly,
  serveEm,
  settings,
  type Scope,
  summary,
  writer,
} from './testkit'

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

interface Backend {
  items?: ItemSummary[]
  categories?: Category[]
  attributes?: Attribute[]
  suppliers?: Supplier[]
  settings?: TenantSettings
}

/** serve answers the reads every open of the screen makes. */
function serve(backend: Backend = {}) {
  server.use(
    http.get('*/readyz', () => HttpResponse.json({ status: 'ok' })),
    http.get('*/api/v1/admin/catalog/items', () => HttpResponse.json({ items: backend.items ?? [summary()] })),
    http.get('*/api/v1/admin/catalog/categories', () => HttpResponse.json({ items: backend.categories ?? [bancos, guidoes] })),
    http.get('*/api/v1/admin/catalog/attributes', () => HttpResponse.json({ items: backend.attributes ?? [serveEm] })),
    http.get('*/api/v1/admin/suppliers', () => HttpResponse.json({ items: backend.suppliers ?? [couros, metais] })),
    http.get('*/api/v1/admin/settings', () => HttpResponse.json(backend.settings ?? settings())),
  )
}

const signedInAs = (role: 'owner' | 'admin' | 'staff' | 'viewer', permissions: Permission[]) =>
  server.use(http.post('*/auth/refresh', () => HttpResponse.json(authSession({ role, permissions }))))

async function openCatalog(name = 'Banco de couro Scrambler') {
  const app = await openApp('/catalogo')
  await vi.waitFor(() => expect(app.wrapper.text()).toContain(name))
  return app
}

/** drawer opens the item's form from its name in the table. */
async function openItem(wrapper: VueWrapper, name = 'Banco de couro Scrambler') {
  await button(wrapper, name).trigger('click')
  await vi.waitFor(() => expect(wrapper.find('[role=dialog]').exists()).toBe(true))
  await flushPromises()
  return wrapper.get('[role=dialog]')
}


beforeEach(() => serve())

describe('the Catálogo menu entry and route', () => {
  it('is in the menu for every role that reads the catalog', async () => {
    signedInAs('viewer', readOnly)

    const { wrapper } = await openApp('/hoje')

    expect(link(wrapper, 'Catálogo').attributes('href')).toBe('/catalogo')
  })

  it('is hidden from a role without catalog:read, and its address leads to Hoje', async () => {
    signedInAs('viewer', ['order:read'])

    const { wrapper, router } = await openApp('/catalogo')

    expect(router.currentRoute.value.name).toBe('hoje')
    expect(wrapper.findAll('a').map((a) => a.text())).not.toContain('Catálogo')
  })
})

describe('the Itens table', () => {
  it('lists items with their category, origin, supplier, lead time and price', async () => {
    signedInAs('admin', writer)
    serve({ items: [summary(), summary({ id: 'x2', name: 'Guidão', categoryId: null, sourcingMode: 'in_house', mainSupplierName: null, readyDays: 1, price: brl(9900) })] })

    const { wrapper } = await openCatalog('Guidão')

    const headers = columnHeaders(wrapper)
    expect(headers).toEqual(['Item', 'Categoria', 'Origem', 'Fornecedor', 'Prazo', 'Preço', 'No portal'])
    const rows = wrapper.findAll('tbody tr').map((row) => plain(row.text()))
    expect(rows[0]).toContain('Bancos')
    expect(rows[0]).toContain('Sob encomenda')
    expect(rows[0]).toContain('Couros Paulista')
    expect(rows[0]).toContain('15 dias')
    expect(rows[0]).toContain('R$ 450,00')
    expect(rows[1]).toContain('Sem categoria')
    expect(rows[1]).toContain('Produção própria')
    expect(rows[1]).toContain('1 dia')
  })

  it('shows Custo and Margem to the owner, because the API sent them', async () => {
    signedInAs('owner', writer)
    serve({ items: [summary(ownerCosts), summary({ id: 'x2', name: 'Sem custo', cost: null, margin: null })] })

    const { wrapper } = await openCatalog()

    expect(columnHeaders(wrapper)).toEqual(['Item', 'Categoria', 'Origem', 'Fornecedor', 'Prazo', 'Custo', 'Preço', 'Margem', 'No portal'])
    const rows = wrapper.findAll('tbody tr').map((row) => plain(row.text()))
    expect(rows[0]).toContain('R$ 315,00')
    expect(rows[0]).toContain('R$ 135,00 (30%)')
    expect(rows[1]).toContain('—')
  })

  it.each([
    ['admin', writer],
    ['viewer', readOnly],
  ] as const)('shows %s neither Custo nor Margem, when the API leaves them out', async (role, permissions) => {
    signedInAs(role, permissions)

    const { wrapper } = await openCatalog()

    const headers = columnHeaders(wrapper)
    expect(headers).not.toContain('Custo')
    expect(headers).not.toContain('Margem')
  })

  it('marks an archived row beside its name', async () => {
    signedInAs('admin', writer)
    serve({ items: [summary({ archived: true, published: false })] })

    const { wrapper } = await openCatalog()

    expect(wrapper.get('tbody tr').text()).toContain('Arquivado')
  })

  it('says when there is nothing, and offers "Novo item" to who can write', async () => {
    signedInAs('admin', writer)
    serve({ items: [] })

    const { wrapper } = await openApp('/catalogo')
    await vi.waitFor(() => expect(wrapper.text()).toContain('Nenhum item no catálogo ainda.'))

    expect(wrapper.findAll('button').filter((b) => b.text() === 'Novo item')).toHaveLength(2)
  })

  it('searches by name, category and archived, and says when nothing matches', async () => {
    signedInAs('admin', writer)
    const queries: URLSearchParams[] = []
    server.use(
      http.get('*/api/v1/admin/catalog/items', ({ request }) => {
        const params = new URL(request.url).searchParams
        queries.push(params)
        return HttpResponse.json({ items: params.get('q') === 'xyz' ? [] : [summary()] })
      }),
    )
    const { wrapper } = await openCatalog()

    await field(wrapper, 'Buscar item').setValue('xyz')
    await vi.waitFor(() => expect(wrapper.text()).toContain('Nenhum item encontrado.'))
    await field(wrapper, 'Filtrar por categoria').setValue(guidoes.id)
    await field(wrapper, 'Mostrar arquivados').setValue(true)
    await vi.waitFor(() => expect(queries.at(-1)?.get('includeArchived')).toBe('true'))

    expect(queries.at(-1)?.get('q')).toBe('xyz')
    expect(queries.at(-1)?.get('categoryId')).toBe(guidoes.id)
    expect(field(wrapper, 'Filtrar por categoria').findAll('option').map((o) => o.text())).toEqual(['Todas as categorias', 'Bancos', 'Guidões'])
  })

  it('loads more with the cursor', async () => {
    signedInAs('admin', writer)
    const cursors: (string | null)[] = []
    server.use(
      http.get('*/api/v1/admin/catalog/items', ({ request }) => {
        const cursor = new URL(request.url).searchParams.get('cursor')
        cursors.push(cursor)
        return cursor === null
          ? HttpResponse.json({ items: [summary()], nextCursor: 'c2' })
          : HttpResponse.json({ items: [summary({ id: 'x2', name: 'Guidão' })] })
      }),
    )
    const { wrapper } = await openCatalog()

    await button(wrapper, 'Carregar mais').trigger('click')
    await vi.waitFor(() => expect(wrapper.text()).toContain('Guidão'))

    expect(cursors).toEqual([null, 'c2'])
    expect(wrapper.text()).not.toContain('Carregar mais')
  })

  it('switches "No portal" with the item version, and reads its name aloud', async () => {
    signedInAs('admin', writer)
    const bodies: unknown[] = []
    server.use(
      http.put(`*/api/v1/admin/catalog/items/${bancoId}/published`, async ({ request }) => {
        bodies.push(await request.json())
        return HttpResponse.json(item({ published: false, version: 4 }))
      }),
    )
    const { wrapper } = await openCatalog()
    const toggle = field(wrapper, 'Mostrar Banco de couro Scrambler na página da loja')
    expect((toggle.element as HTMLInputElement).checked).toBe(true)

    await toggle.setValue(false)
    await flushPromises()

    expect(bodies).toEqual([{ published: false, version: 3 }])
    expect((field(wrapper, 'Mostrar Banco de couro Scrambler na página da loja').element as HTMLInputElement).checked).toBe(false)
  })

  it('leaves "No portal" as it was when the change is refused, and says why', async () => {
    signedInAs('admin', writer)
    server.use(http.put(`*/api/v1/admin/catalog/items/${bancoId}/published`, () => problem(409, 'version_conflict', 'Conflito')))
    const { wrapper } = await openCatalog()

    await field(wrapper, 'Mostrar Banco de couro Scrambler na página da loja').setValue(false)
    await flushPromises()

    await vi.waitFor(() => expect(wrapper.get('[role=alert]').text()).toBe('Outra pessoa alterou este item. Recarregue para ver a versão atual.'))
    expect((field(wrapper, 'Mostrar Banco de couro Scrambler na página da loja').element as HTMLInputElement).checked).toBe(true)
  })
})

describe('a role that cannot write', () => {
  it('sees no editing control, in the table or in the item', async () => {
    signedInAs('viewer', readOnly)
    server.use(http.get(`*/api/v1/admin/catalog/items/${bancoId}`, () => HttpResponse.json(item({ photos: [{ id: 'p1', url: 'http://s.test/p1', expiresAt: '2026-12-01T00:00:00Z', position: 0 }] }))))
    const { wrapper } = await openCatalog()
    const names = wrapper.findAll('button').map((b) => b.text())
    expect(names).not.toContain('Novo item')
    expect((field(wrapper, 'Mostrar Banco de couro Scrambler na página da loja').element as HTMLInputElement).disabled).toBe(true)

    const drawer = await openItem(wrapper)

    expect(drawer.get('h2').text()).toBe('Item')
    const inside = drawer.findAll('button').map((b) => b.text())
    expect(inside).toEqual(['Fechar'])
    expect(drawer.findAll('fieldset[disabled]').length).toBeGreaterThan(0)
    expect(drawer.text()).not.toContain('Adicionar fotos')
    expect(drawer.text()).not.toContain('Arquivar')
  })

  it('opens the categories and fields dialogs to read them, without their buttons', async () => {
    signedInAs('staff', readOnly)
    const { wrapper } = await openCatalog()

    await button(wrapper, 'Categorias').trigger('click')
    expect(wrapper.get('[role=dialog]').text()).toContain('Guidões')
    expect(wrapper.get('[role=dialog]').text()).not.toContain('Nova categoria')
    await button(wrapper.get('[role=dialog]'), 'Fechar').trigger('click')
    await button(wrapper, 'Campos').trigger('click')

    expect((field(wrapper.get('[role=dialog]'), 'Valor 1').element as HTMLInputElement).value).toBe('CB 500X')
    expect(wrapper.get('[role=dialog]').text()).not.toContain('Novo campo')
  })
})

describe('the item form', () => {
  it('offers Estoque and Produção própria only when the store turned them on', async () => {
    signedInAs('admin', writer)
    const { wrapper } = await openCatalog()
    await button(wrapper, 'Novo item').trigger('click')
    expect(field(wrapper, 'Origem').findAll('option').map((o) => o.text())).toEqual(['Sob encomenda'])
    await button(wrapper.get('[role=dialog]'), 'Cancelar').trigger('click')

    serve({ settings: settings({ stockEnabled: true, productionEnabled: true }) })
    const second = await openCatalog()
    await button(second.wrapper, 'Novo item').trigger('click')

    expect(field(second.wrapper, 'Origem').findAll('option').map((o) => o.text())).toEqual(['Sob encomenda', 'Estoque', 'Produção própria'])
  })

  it('titles a new item and an edited one', async () => {
    signedInAs('admin', writer)
    server.use(http.get(`*/api/v1/admin/catalog/items/${bancoId}`, () => HttpResponse.json(item())))
    const { wrapper } = await openCatalog()

    await button(wrapper, 'Novo item').trigger('click')
    expect(wrapper.get('[role=dialog] h2').text()).toBe('Novo item')
    await button(wrapper.get('[role=dialog]'), 'Cancelar').trigger('click')
    const drawer = await openItem(wrapper)

    expect(drawer.get('h2').text()).toBe('Editar item')
    expect((field(drawer, 'Nome').element as HTMLInputElement).value).toBe('Banco de couro Scrambler')
    expect((field(drawer, 'Preço').element as HTMLInputElement).value).toBe('450,00')
    expect((field(drawer, 'Prazo do fornecedor (dias)').element as HTMLInputElement).value).toBe('15')
  })

  it('creates an item with its supplier, price in cents, fields and options', async () => {
    signedInAs('admin', writer)
    const bodies: Record<string, unknown>[] = []
    server.use(
      http.post('*/api/v1/admin/catalog/items', async ({ request }) => {
        bodies.push((await request.json()) as Record<string, unknown>)
        return HttpResponse.json(item(), { status: 201 })
      }),
    )
    const { wrapper } = await openCatalog()
    await button(wrapper, 'Novo item').trigger('click')
    const drawer = wrapper.get('[role=dialog]')

    await field(drawer, 'Nome').setValue('  Banco novo ')
    await field(drawer, 'Preço').setValue('1.234,50')
    await field(drawer, 'Categoria').setValue(bancos.id)
    await field(drawer, 'Fornecedor principal').setValue(couros.id)
    await field(drawer, 'Prazo do fornecedor (dias)').setValue('12')
    await field(drawer, 'CB 500X').setValue(true)
    await button(drawer, 'Nova opção').trigger('click')
    await field(drawer, 'Nome da opção').setValue('Cor da costura')
    await field(drawer, 'Escolha').setValue('Vermelha')
    await field(drawer, 'Acréscimo no preço').setValue('50')
    await field(drawer, 'Dias a mais').setValue('2')
    await drawer.get('form').trigger('submit')
    await flushPromises()

    expect(bodies).toEqual([
      {
        name: 'Banco novo',
        description: '',
        categoryId: bancos.id,
        sourcingMode: 'to_order',
        priceCents: 123450,
        productionDays: null,
        mainSupplier: { supplierId: couros.id, leadDays: 12 },
        backupSupplier: null,
        attributeValueIds: [serveEm.values[0]?.id],
        options: [{ name: 'Cor da costura', required: false, choices: [{ label: 'Vermelha', extraPriceCents: 5000, extraDays: 2 }] }],
        published: false,
      },
    ])
    expect(wrapper.get('[role=status]').text()).toBe('Item salvo.')
    expect(wrapper.find('[role=dialog]').exists()).toBe(false)
  })

  it('names the empty fields before calling the API', async () => {
    signedInAs('admin', writer)
    const { wrapper } = await openCatalog()
    await button(wrapper, 'Novo item').trigger('click')
    const drawer = wrapper.get('[role=dialog]')

    await drawer.get('form').trigger('submit')
    await flushPromises()

    expect(drawer.get('[role=alert]').text()).toBe('Confira os campos destacados.')
    expect(drawer.text()).toContain('Informe o nome do item.')
    expect(drawer.text()).toContain('Informe o preço.')
    expect(drawer.text()).toContain('Escolha o fornecedor principal.')
    expect(drawer.text()).toContain('Informe o prazo do fornecedor.')
  })

  it('catches the lead time and the same reserve supplier in the form', async () => {
    signedInAs('admin', writer)
    server.use(
      http.post('*/api/v1/admin/catalog/items', () =>
        problem(400, 'invalid', 'Dados inválidos', {
          errors: [
            { field: 'priceCents', message: 'Informe o preço.' },
            { field: 'mainSupplier.leadDays', message: 'Use um prazo de 0 a 365 dias.' },
            { field: 'backupSupplier.supplierId', message: 'O fornecedor reserva precisa ser outro.' },
            { field: 'options[0].choices[1].label', message: 'Já existe uma escolha com este nome.' },
          ],
        }),
      ),
    )
    const { wrapper } = await openCatalog()
    await button(wrapper, 'Novo item').trigger('click')
    const drawer = wrapper.get('[role=dialog]')
    await field(drawer, 'Nome').setValue('Banco')
    await field(drawer, 'Preço').setValue('10')
    await field(drawer, 'Fornecedor principal').setValue(couros.id)
    await field(drawer, 'Prazo do fornecedor (dias)').setValue('400')
    await button(drawer, 'Adicionar fornecedor reserva').trigger('click')
    await field(group(drawer, 'Fornecedor reserva'), 'Fornecedor reserva').setValue(couros.id)
    await field(group(drawer, 'Fornecedor reserva'), 'Prazo do fornecedor (dias)').setValue('5')
    await drawer.get('form').trigger('submit')
    await flushPromises()

    // The form itself catches the reserve being the same, and the lead time out of range.
    expect(group(drawer, 'Fornecedor').text()).toContain('Use um prazo de 0 a 365 dias.')
    expect(group(drawer, 'Fornecedor reserva').text()).toContain('O fornecedor reserva precisa ser outro.')
    expect(drawer.get('[role=alert]').text()).toBe('Confira os campos destacados.')
  })

  it('shows the field messages the API sends next to their fields', async () => {
    signedInAs('admin', writer)
    server.use(
      http.post('*/api/v1/admin/catalog/items', () =>
        problem(400, 'invalid', 'Dados inválidos', {
          errors: [
            { field: 'priceCents', message: 'Informe o preço.' },
            { field: 'mainSupplier.leadDays', message: 'Use um prazo de 0 a 365 dias.' },
            { field: 'options[0].choices[1].label', message: 'Já existe uma escolha com este nome.' },
          ],
        }),
      ),
    )
    const { wrapper } = await openCatalog()
    await button(wrapper, 'Novo item').trigger('click')
    const drawer = wrapper.get('[role=dialog]')
    await field(drawer, 'Nome').setValue('Banco')
    await field(drawer, 'Preço').setValue('10')
    await field(drawer, 'Fornecedor principal').setValue(couros.id)
    await field(drawer, 'Prazo do fornecedor (dias)').setValue('12')
    await button(drawer, 'Nova opção').trigger('click')
    await field(drawer, 'Nome da opção').setValue('Cor')
    await field(drawer, 'Escolha').setValue('Azul')
    await button(drawer, 'Adicionar escolha').trigger('click')
    await field(group(drawer, 'Escolha 2'), 'Escolha').setValue('Vermelha')
    await drawer.get('form').trigger('submit')
    await flushPromises()

    expect(drawer.get('[role=alert]').text()).toBe('Confira os campos destacados.')
    expect(field(drawer, 'Preço').attributes('aria-invalid')).toBe('true')
    expect(drawer.text()).toContain('Informe o preço.')
    expect(group(drawer, 'Fornecedor').text()).toContain('Use um prazo de 0 a 365 dias.')
    expect(group(drawer, 'Escolha 2').text()).toContain('Já existe uma escolha com este nome.')
  })

  it('creates a supplier inline and chooses it', async () => {
    signedInAs('admin', writer)
    const bodies: unknown[] = []
    const created: Supplier = { id: '0190f3c4-5e1a-7b2c-8d3e-0000000000f3', name: 'Aço Norte', whatsappPhone: '5511987654321', version: 1 }
    server.use(
      http.post('*/api/v1/admin/suppliers', async ({ request }) => {
        bodies.push(await request.json())
        return HttpResponse.json(created, { status: 201 })
      }),
    )
    const { wrapper } = await openCatalog()
    await button(wrapper, 'Novo item').trigger('click')
    const drawer = wrapper.get('[role=dialog]')

    await field(drawer, 'Fornecedor principal').setValue('new')
    await field(drawer, 'Nome do fornecedor').setValue(' Aço Norte ')
    await field(drawer, 'WhatsApp do fornecedor').setValue('(11) 98765-4321')
    await button(drawer, 'Adicionar fornecedor').trigger('click')
    await flushPromises()

    expect(bodies).toEqual([{ name: 'Aço Norte', whatsappPhone: '5511987654321' }])
    const select = field(drawer, 'Fornecedor principal')
    expect((select.element as HTMLSelectElement).value).toBe(created.id)
    expect(select.findAll('option').map((o) => o.text())).toContain('Aço Norte')
    expect(drawer.text()).not.toContain('WhatsApp do fornecedor')
  })

  it('checks the new supplier before creating it', async () => {
    signedInAs('admin', writer)
    const { wrapper } = await openCatalog()
    await button(wrapper, 'Novo item').trigger('click')
    const drawer = wrapper.get('[role=dialog]')
    await field(drawer, 'Fornecedor principal').setValue('new')

    await button(drawer, 'Adicionar fornecedor').trigger('click')
    expect(drawer.text()).toContain('Informe o nome do fornecedor.')
    expect(drawer.text()).toContain('Informe o WhatsApp do fornecedor.')
    await field(drawer, 'Nome do fornecedor').setValue('Aço Norte')
    await field(drawer, 'WhatsApp do fornecedor').setValue('1234')
    await button(drawer, 'Adicionar fornecedor').trigger('click')

    expect(drawer.text()).toContain('Confira o número: use DDD e 8 ou 9 dígitos.')
  })

  it('adds and removes a backup supplier', async () => {
    signedInAs('admin', writer)
    const { wrapper } = await openCatalog()
    await button(wrapper, 'Novo item').trigger('click')
    const drawer = wrapper.get('[role=dialog]')

    await button(drawer, 'Adicionar fornecedor reserva').trigger('click')
    expect(group(drawer, 'Fornecedor reserva').exists()).toBe(true)
    await button(drawer, 'Remover fornecedor reserva').trigger('click')

    expect(drawer.findAll('legend').map((l) => l.text())).not.toContain('Fornecedor reserva')
  })

  it('shows production fields for an in-house item', async () => {
    signedInAs('owner', writer)
    serve({ settings: settings({ productionEnabled: true }) })
    const { wrapper } = await openCatalog()
    await button(wrapper, 'Novo item').trigger('click')
    const drawer = wrapper.get('[role=dialog]')

    await field(drawer, 'Origem').setValue('in_house')

    expect(field(drawer, 'Dias de produção').element).toBeTruthy()
    expect(field(drawer, 'Custo de produção').element).toBeTruthy()
    expect(drawer.text()).not.toContain('Fornecedor principal')
  })
})

describe('costs in the item form (ADR 0025)', () => {
  const putBodies: Record<string, unknown>[] = []
  beforeEach(() => {
    putBodies.length = 0
    server.use(
      http.put(`*/api/v1/admin/catalog/items/${bancoId}`, async ({ request }) => {
        putBodies.push((await request.json()) as Record<string, unknown>)
        return HttpResponse.json(item())
      }),
    )
  })

  async function save(wrapper: VueWrapper) {
    await wrapper.get('[role=dialog] form').trigger('submit')
    await flushPromises()
  }

  it('never sends a cost key for an admin, and shows no Custo field', async () => {
    signedInAs('admin', writer)
    server.use(http.get(`*/api/v1/admin/catalog/items/${bancoId}`, () => HttpResponse.json(item({ backupSupplier: { supplierId: metais.id, supplierName: metais.name, leadDays: 9 } }))))
    const { wrapper } = await openCatalog()
    const drawer = await openItem(wrapper)
    expect(drawer.findAll('label').map((l) => l.text())).not.toContain('Custo')

    await field(drawer, 'Nome').setValue('Banco renomeado')
    await save(wrapper)

    expect(putBodies).toHaveLength(1)
    expect(JSON.stringify(putBodies[0])).not.toMatch(/cost/i)
    expect(putBodies[0]).toMatchObject({
      name: 'Banco renomeado',
      version: 3,
      mainSupplier: { supplierId: couros.id, leadDays: 15 },
      backupSupplier: { supplierId: metais.id, leadDays: 9 },
    })
  })

  it('sends the owner no cost key either, until the owner changes one', async () => {
    signedInAs('owner', writer)
    server.use(http.get(`*/api/v1/admin/catalog/items/${bancoId}`, () => HttpResponse.json(ownerItem())))
    const { wrapper } = await openCatalog()
    const drawer = await openItem(wrapper)
    expect((field(drawer, 'Custo').element as HTMLInputElement).value).toBe('315,00')

    await field(drawer, 'Nome').setValue('Banco renomeado')
    await save(wrapper)

    expect(JSON.stringify(putBodies[0])).not.toMatch(/cost/i)
  })

  it('sends the cents of a cost the owner changed, and null for one cleared', async () => {
    signedInAs('owner', writer)
    server.use(http.get(`*/api/v1/admin/catalog/items/${bancoId}`, () => HttpResponse.json(ownerItem())))
    const { wrapper } = await openCatalog()
    const drawer = await openItem(wrapper)

    await field(drawer, 'Custo').setValue('300,50')
    await save(wrapper)
    expect(putBodies[0]).toMatchObject({ mainSupplier: { supplierId: couros.id, costCents: 30050 } })

    const again = await openItem(wrapper)
    await field(again, 'Custo').setValue('')
    await save(wrapper)
    expect(putBodies[1]).toMatchObject({ mainSupplier: { supplierId: couros.id, costCents: null } })
  })

  it('starts a newly chosen supplier with no cost, and brings back the cost of one the item had', async () => {
    signedInAs('owner', writer)
    server.use(http.get(`*/api/v1/admin/catalog/items/${bancoId}`, () => HttpResponse.json(ownerItem())))
    const { wrapper } = await openCatalog()
    const drawer = await openItem(wrapper)

    await field(drawer, 'Fornecedor principal').setValue(metais.id)
    expect((field(drawer, 'Custo').element as HTMLInputElement).value).toBe('')
    await field(drawer, 'Fornecedor principal').setValue(couros.id)

    expect((field(drawer, 'Custo').element as HTMLInputElement).value).toBe('315,00')
  })

  it('sends the production cost of an in-house item only when the owner changed it', async () => {
    signedInAs('owner', writer)
    serve({ settings: settings({ productionEnabled: true }) })
    const inHouse = item({ sourcingMode: 'in_house', productionDays: 4, mainSupplier: null, productionCost: brl(8000), cost: brl(8000), margin: { cents: 37000, basisPoints: 8222 } })
    server.use(http.get(`*/api/v1/admin/catalog/items/${bancoId}`, () => HttpResponse.json(inHouse)))
    const { wrapper } = await openCatalog()
    const drawer = await openItem(wrapper)
    expect((field(drawer, 'Custo de produção').element as HTMLInputElement).value).toBe('80,00')

    await save(wrapper)
    expect(JSON.stringify(putBodies[0])).not.toMatch(/cost/i)
    expect(putBodies[0]).toMatchObject({ sourcingMode: 'in_house', productionDays: 4, mainSupplier: null })

    const again = await openItem(wrapper)
    await field(again, 'Custo de produção').setValue('90')
    await save(wrapper)
    expect(putBodies[1]).toMatchObject({ productionCostCents: 9000 })
  })
})

describe('archiving, restoring and conflicts', () => {
  it('archives an item and says so', async () => {
    signedInAs('admin', writer)
    server.use(
      http.get(`*/api/v1/admin/catalog/items/${bancoId}`, () => HttpResponse.json(item())),
      http.post(`*/api/v1/admin/catalog/items/${bancoId}/archive`, async ({ request }) => {
        expect(await request.json()).toEqual({ version: 3 })
        return HttpResponse.json(item({ archived: true, published: false, version: 4 }))
      }),
    )
    const { wrapper } = await openCatalog()
    const drawer = await openItem(wrapper)

    await button(drawer, 'Arquivar').trigger('click')
    await flushPromises()

    expect(wrapper.get('[role=status]').text()).toBe('Banco de couro Scrambler foi arquivado.')
    expect(wrapper.find('[role=dialog]').exists()).toBe(false)
  })

  it('names the kit that holds the item, and keeps the form open', async () => {
    signedInAs('admin', writer)
    server.use(
      http.get(`*/api/v1/admin/catalog/items/${bancoId}`, () => HttpResponse.json(item())),
      http.post(`*/api/v1/admin/catalog/items/${bancoId}/archive`, () =>
        problem(409, 'item_in_kit', 'Item em kit', { references: [{ id: 'k1', name: 'Kit Scrambler CB 500X' }] }),
      ),
    )
    const { wrapper } = await openCatalog()
    const drawer = await openItem(wrapper)

    await button(drawer, 'Arquivar').trigger('click')
    await flushPromises()

    expect(drawer.get('[role=alert]').text()).toBe('Este item está no kit Kit Scrambler CB 500X. Tire-o do kit antes de arquivar.')
    expect(wrapper.find('[role=dialog]').exists()).toBe(true)
  })

  it('names every kit when there are several', async () => {
    signedInAs('admin', writer)
    server.use(
      http.get(`*/api/v1/admin/catalog/items/${bancoId}`, () => HttpResponse.json(item())),
      http.post(`*/api/v1/admin/catalog/items/${bancoId}/archive`, () =>
        problem(409, 'item_in_kit', 'Item em kit', {
          references: [
            { id: 'k1', name: 'Kit A' },
            { id: 'k2', name: 'Kit B' },
            { id: 'k3', name: 'Kit C' },
          ],
        }),
      ),
    )
    const { wrapper } = await openCatalog()
    const drawer = await openItem(wrapper)

    await button(drawer, 'Arquivar').trigger('click')
    await flushPromises()

    expect(drawer.get('[role=alert]').text()).toBe('Este item está nos kits Kit A, Kit B e Kit C. Tire-o dos kits antes de arquivar.')
  })

  it('restores an archived item', async () => {
    signedInAs('admin', writer)
    server.use(
      http.get(`*/api/v1/admin/catalog/items/${bancoId}`, () => HttpResponse.json(item({ archived: true, published: false }))),
      http.post(`*/api/v1/admin/catalog/items/${bancoId}/restore`, () => HttpResponse.json(item())),
    )
    serve({ items: [summary({ archived: true, published: false })] })
    const { wrapper } = await openCatalog()
    const drawer = await openItem(wrapper)
    expect(drawer.text()).not.toContain('Arquivar')

    await button(drawer, 'Restaurar').trigger('click')
    await flushPromises()

    expect(wrapper.get('[role=status]').text()).toBe('Banco de couro Scrambler voltou ao catálogo.')
  })

  it('says someone else saved first, and reloads the item on "Recarregar"', async () => {
    signedInAs('admin', writer)
    let reads = 0
    server.use(
      http.get(`*/api/v1/admin/catalog/items/${bancoId}`, () => {
        reads += 1
        return HttpResponse.json(item(reads === 1 ? {} : { name: 'Banco da outra pessoa', version: 4 }))
      }),
      http.put(`*/api/v1/admin/catalog/items/${bancoId}`, () => problem(409, 'version_conflict', 'Conflito')),
    )
    const { wrapper } = await openCatalog()
    const drawer = await openItem(wrapper)
    await field(drawer, 'Nome').setValue('Meu nome')

    await drawer.get('form').trigger('submit')
    await flushPromises()

    expect(drawer.get('[role=alert]').text()).toContain('Outra pessoa alterou este item. Recarregue para ver a versão atual.')
    await button(drawer, 'Recarregar').trigger('click')
    await flushPromises()

    expect((field(wrapper.get('[role=dialog]'), 'Nome').element as HTMLInputElement).value).toBe('Banco da outra pessoa')
    expect(wrapper.get('[role=dialog]').text()).not.toContain('Recarregar')
  })

  it('says it is unreachable when the server cannot be', async () => {
    signedInAs('admin', writer)
    server.use(
      http.get(`*/api/v1/admin/catalog/items/${bancoId}`, () => HttpResponse.json(item())),
      http.put(`*/api/v1/admin/catalog/items/${bancoId}`, () => HttpResponse.error()),
    )
    const { wrapper } = await openCatalog()
    const drawer = await openItem(wrapper)

    await drawer.get('form').trigger('submit')
    await flushPromises()

    expect(drawer.get('[role=alert]').text()).toBe('Sem conexão com o servidor. Confira a internet e tente de novo.')
  })
})

describe('photos', () => {
  const photo = (id: string, position: number): Photo => ({ id, url: `http://storage.test/${id}`, expiresAt: '2026-12-01T00:00:00Z', position })
  const jpeg = (size = 10) => {
    const file = new File(['x'], 'banco.jpg', { type: 'image/jpeg' })
    Object.defineProperty(file, 'size', { value: size })
    return file
  }

  async function choose(drawer: Scope, file: File) {
    const input = field(drawer, 'Adicionar fotos')
    Object.defineProperty(input.element, 'files', { value: [file], configurable: true })
    await input.trigger('change')
    await flushPromises()
  }

  function serveUpload(confirm: () => Response) {
    server.use(
      http.get(`*/api/v1/admin/catalog/items/${bancoId}`, () => HttpResponse.json(item({ photos: [photo('p1', 0)] }))),
      http.post('*/api/v1/admin/catalog/photo-uploads', () =>
        HttpResponse.json({ key: 'k1', uploadUrl: 'http://storage.test/up/k1', method: 'PUT', headers: { 'Content-Type': 'image/jpeg' }, expiresAt: '2026-12-01T00:00:00Z' }, { status: 201 }),
      ),
      http.put('http://storage.test/up/k1', () => new HttpResponse(null, { status: 200 })),
      http.post(`*/api/v1/admin/catalog/items/${bancoId}/photos`, confirm),
    )
  }

  it('uploads a photo and shows it after the first, which is the cover', async () => {
    signedInAs('admin', writer)
    serveUpload(() => HttpResponse.json(photo('p2', 1), { status: 201 }))
    const { wrapper } = await openCatalog()
    const drawer = await openItem(wrapper)
    expect(drawer.text()).toContain('JPEG, PNG ou WebP, até 10 MB. A primeira é a capa.')

    await choose(drawer, jpeg())

    expect(drawer.findAll('img').map((img) => img.attributes('alt'))).toEqual(['Foto 1', 'Foto 2'])
    expect(drawer.findAll('[role=alert]')).toHaveLength(0)
  })

  it('says a photo the server refused is not a JPEG, PNG or WebP of up to 10 MB', async () => {
    signedInAs('admin', writer)
    serveUpload(() => problem(400, 'photo_invalid', 'Foto inválida'))
    const { wrapper } = await openCatalog()
    const drawer = await openItem(wrapper)

    await choose(drawer, jpeg())

    expect(drawer.get('[role=alert]').text()).toBe('Use uma foto JPEG, PNG ou WebP de até 10 MB.')
    expect(drawer.findAll('img')).toHaveLength(1)
  })

  it('refuses a photo over 10 MB without asking the server', async () => {
    signedInAs('admin', writer)
    server.use(http.get(`*/api/v1/admin/catalog/items/${bancoId}`, () => HttpResponse.json(item())))
    // No upload handler: any request would fail the test as unhandled.
    const { wrapper } = await openCatalog()
    const drawer = await openItem(wrapper)

    await choose(drawer, jpeg(11 * 1024 * 1024))

    expect(drawer.get('[role=alert]').text()).toBe('Esta foto passa de 10 MB.')
  })

  it('refuses a file that is not an image of a known type', async () => {
    signedInAs('admin', writer)
    server.use(http.get(`*/api/v1/admin/catalog/items/${bancoId}`, () => HttpResponse.json(item())))
    const { wrapper } = await openCatalog()
    const drawer = await openItem(wrapper)

    await choose(drawer, new File(['x'], 'nota.pdf', { type: 'application/pdf' }))

    expect(drawer.get('[role=alert]').text()).toBe('Use uma foto JPEG, PNG ou WebP.')
  })

  it('says the photo was not sent when storage refuses it, and when the item is at 8 photos', async () => {
    signedInAs('admin', writer)
    serveUpload(() => HttpResponse.json(photo('p2', 1), { status: 201 }))
    server.use(http.put('http://storage.test/up/k1', () => new HttpResponse(null, { status: 403 })))
    const { wrapper } = await openCatalog()
    const drawer = await openItem(wrapper)

    await choose(drawer, jpeg())
    expect(drawer.get('[role=alert]').text()).toBe('A foto não foi enviada. Tente de novo.')

    server.use(http.post('*/api/v1/admin/catalog/photo-uploads', () => problem(409, 'photo_limit', 'Limite')))
    await choose(drawer, jpeg())
    expect(drawer.get('[role=alert]').text()).toBe('Cada item pode ter até 8 fotos.')
  })

  it('makes a photo the cover and removes one', async () => {
    signedInAs('admin', writer)
    const orders: unknown[] = []
    server.use(
      http.get(`*/api/v1/admin/catalog/items/${bancoId}`, () => HttpResponse.json(item({ photos: [photo('p1', 0), photo('p2', 1), photo('p3', 2)] }))),
      http.put(`*/api/v1/admin/catalog/items/${bancoId}/photos/order`, async ({ request }) => {
        const body = (await request.json()) as { photoIds: string[] }
        orders.push(body)
        return HttpResponse.json({ items: body.photoIds.map((id, position) => photo(id, position)) })
      }),
      http.delete(`*/api/v1/admin/catalog/items/${bancoId}/photos/p1`, () => new HttpResponse(null, { status: 204 })),
    )
    const { wrapper } = await openCatalog()
    const drawer = await openItem(wrapper)

    await button(drawer, 'Usar como capa').trigger('click')
    await flushPromises()
    expect(orders).toEqual([{ photoIds: ['p2', 'p1', 'p3'] }])

    await drawer.findAll('button').filter((b) => b.text() === 'Remover foto')[1]?.trigger('click')
    await flushPromises()
    expect(drawer.findAll('img')).toHaveLength(2)
  })

  it('asks a new item to be saved before it takes photos', async () => {
    signedInAs('admin', writer)
    const { wrapper } = await openCatalog()

    await button(wrapper, 'Novo item').trigger('click')

    expect(wrapper.get('[role=dialog]').text()).toContain('Salve o item para adicionar fotos.')
  })
})

describe('the Categorias dialog', () => {
  it('lists the categories in order and moves one', async () => {
    signedInAs('admin', writer)
    const patches: unknown[] = []
    server.use(
      http.patch(`*/api/v1/admin/catalog/categories/${guidoes.id}`, async ({ request }) => {
        patches.push(await request.json())
        return HttpResponse.json({ ...guidoes, position: 0 })
      }),
    )
    const { wrapper } = await openCatalog()
    await button(wrapper, 'Categorias').trigger('click')
    const dialog = wrapper.get('[role=dialog]')
    const rows = dialog.findAll('li')
    expect(rows).toHaveLength(2)
    expect(rows[0]?.text()).toContain('Bancos')
    expect(rows[1]?.text()).toContain('Guidões')
    expect(button(dialog, 'Mover Bancos para cima').attributes('disabled')).toBeDefined()

    await button(dialog, 'Mover Guidões para cima').trigger('click')
    await flushPromises()

    expect(patches).toEqual([{ position: 0 }])
  })

  it('adds a category, and says when the name is taken', async () => {
    signedInAs('admin', writer)
    server.use(http.post('*/api/v1/admin/catalog/categories', () => problem(409, 'name_taken', 'Nome em uso')))
    const { wrapper } = await openCatalog()
    await button(wrapper, 'Categorias').trigger('click')
    const dialog = wrapper.get('[role=dialog]')

    await button(dialog, 'Nova categoria').trigger('click')
    await flushPromises()
    expect(dialog.text()).toContain('Informe o nome da categoria.')
    await field(dialog, 'Nome da categoria').setValue('Bancos')
    await button(dialog, 'Nova categoria').trigger('click')
    await flushPromises()

    expect(dialog.text()).toContain('Já existe uma categoria com este nome.')
  })

  it('renames a category', async () => {
    signedInAs('admin', writer)
    const patches: unknown[] = []
    server.use(
      http.patch(`*/api/v1/admin/catalog/categories/${bancos.id}`, async ({ request }) => {
        patches.push(await request.json())
        return HttpResponse.json({ ...bancos, name: 'Assentos' })
      }),
    )
    const { wrapper } = await openCatalog()
    await button(wrapper, 'Categorias').trigger('click')
    const dialog = wrapper.get('[role=dialog]')

    await button(dialog, 'Renomear Bancos').trigger('click')
    await field(dialog, 'Novo nome de Bancos').setValue('Assentos')
    await button(dialog, 'Salvar').trigger('click')
    await flushPromises()

    expect(patches).toEqual([{ name: 'Assentos' }])
    await vi.waitFor(() => expect(dialog.text()).not.toContain('Novo nome de'))
  })

  it('refuses to delete a category items still use, and says what to do', async () => {
    signedInAs('admin', writer)
    server.use(http.delete(`*/api/v1/admin/catalog/categories/${bancos.id}`, () => problem(409, 'category_in_use', 'Categoria em uso')))
    const { wrapper } = await openCatalog()
    await button(wrapper, 'Categorias').trigger('click')
    const dialog = wrapper.get('[role=dialog]')

    await button(dialog, 'Excluir Bancos').trigger('click')
    await flushPromises()

    expect(dialog.get('[role=alert]').text()).toBe('Esta categoria tem itens. Mude os itens de categoria antes de excluir.')
    expect(dialog.text()).toContain('Bancos')
  })

  it('deletes a category nothing uses', async () => {
    signedInAs('admin', writer)
    let deleted = false
    server.use(
      http.delete(`*/api/v1/admin/catalog/categories/${guidoes.id}`, () => {
        deleted = true
        return new HttpResponse(null, { status: 204 })
      }),
      http.get('*/api/v1/admin/catalog/categories', () => HttpResponse.json({ items: deleted ? [bancos] : [bancos, guidoes] })),
    )
    const { wrapper } = await openCatalog()
    await button(wrapper, 'Categorias').trigger('click')
    const dialog = wrapper.get('[role=dialog]')

    await button(dialog, 'Excluir Guidões').trigger('click')
    await flushPromises()

    await vi.waitFor(() => expect(dialog.findAll('li')).toHaveLength(1))
  })
})

describe('the Campos dialog', () => {
  async function openFields() {
    const app = await openCatalog()
    await button(app.wrapper, 'Campos').trigger('click')
    return { ...app, dialog: app.wrapper.get('[role=dialog]') }
  }

  it('lists each field with its values', async () => {
    signedInAs('admin', writer)

    const { dialog } = await openFields()

    expect(dialog.get('h2').text()).toBe('Campos dos itens')
    expect((field(dialog, 'Nome do campo').element as HTMLInputElement).value).toBe('Serve em')
    expect((field(dialog, 'Valor 1').element as HTMLInputElement).value).toBe('CB 500X')
    expect(dialog.text()).toContain('Campos como "Serve em" ajudam o cliente a achar o que serve para ele.')
  })

  it('asks before removing a value in use, with how many items and kits lose it', async () => {
    signedInAs('admin', writer)
    const bodies: unknown[] = []
    server.use(
      http.put(`*/api/v1/admin/catalog/attributes/${serveEm.id}`, async ({ request }) => {
        bodies.push(await request.json())
        return HttpResponse.json(serveEm)
      }),
    )
    const { dialog } = await openFields()

    await button(dialog, 'Remover valor CB 500X').trigger('click')
    expect(dialog.get('[role=group]').text()).toContain('Remover CB 500X? Ele sai de 3 itens e kits.')
    await button(dialog.get('[role=group]'), 'Cancelar').trigger('click')
    expect(dialog.find('[role=group]').exists()).toBe(false)
    expect(bodies).toEqual([])

    await button(dialog, 'Remover valor CB 500X').trigger('click')
    await button(dialog.get('[role=group]'), 'Remover').trigger('click')
    await button(dialog, 'Salvar campo').trigger('click')
    await flushPromises()

    expect(bodies).toEqual([{ name: 'Serve em', values: [{ id: serveEm.values[1]?.id, label: 'XRE 300' }] }])
  })

  it('removes a value nothing uses without asking', async () => {
    signedInAs('admin', writer)
    const { dialog } = await openFields()

    await button(dialog, 'Remover valor XRE 300').trigger('click')

    expect(dialog.find('[role=group]').exists()).toBe(false)
    expect(dialog.findAll('input').map((input) => (input.element as HTMLInputElement).value)).toEqual(['Serve em', 'CB 500X'])
  })

  it('adds a field with a value, and checks the names', async () => {
    signedInAs('admin', writer)
    const bodies: unknown[] = []
    server.use(
      http.post('*/api/v1/admin/catalog/attributes', async ({ request }) => {
        bodies.push(await request.json())
        return HttpResponse.json({ id: 'a2', name: 'Cor', position: 1, values: [] }, { status: 201 })
      }),
    )
    const { dialog } = await openFields()

    await button(dialog, 'Novo campo').trigger('click')
    const cards = dialog.findAll('form')
    const card = cards[cards.length - 1]
    if (card === undefined) throw new Error('no new card')
    await button(card, 'Salvar campo').trigger('click')
    await flushPromises()
    expect(card.text()).toContain('Informe o nome do campo.')
    expect(card.text()).toContain('Informe o valor.')
    await field(card, 'Nome do campo').setValue('Cor')
    await field(card, 'Valor 1').setValue('Preto')
    await button(card, 'Adicionar valor').trigger('click')
    await field(card, 'Valor 2').setValue('preto')
    await button(card, 'Salvar campo').trigger('click')
    await flushPromises()
    expect(card.text()).toContain('Este valor já existe neste campo.')
    await field(card, 'Valor 2').setValue('Branco')
    await button(card, 'Salvar campo').trigger('click')
    await flushPromises()

    expect(bodies).toEqual([{ name: 'Cor', values: [{ label: 'Preto' }, { label: 'Branco' }] }])
  })

  it('asks before deleting a field whose values are in use', async () => {
    signedInAs('admin', writer)
    let deleted = false
    server.use(
      http.delete(`*/api/v1/admin/catalog/attributes/${serveEm.id}`, () => {
        deleted = true
        return new HttpResponse(null, { status: 204 })
      }),
    )
    const { dialog } = await openFields()

    await button(dialog, 'Excluir campo Serve em').trigger('click')
    expect(dialog.get('[role=group]').text()).toContain('Remover Serve em? Seus valores saem dos itens e kits que os usam.')
    expect(deleted).toBe(false)
    await button(dialog.get('[role=group]'), 'Remover').trigger('click')
    await flushPromises()

    expect(deleted).toBe(true)
  })
})

describe('unsaved edits in the item form', () => {
  async function dirtyDrawer() {
    signedInAs('admin', writer)
    server.use(http.get(`*/api/v1/admin/catalog/items/${bancoId}`, () => HttpResponse.json(item())))
    const { wrapper } = await openCatalog()
    const drawer = await openItem(wrapper)
    await field(drawer, 'Nome').setValue('Meu nome')
    return { wrapper, drawer }
  }

  it('closes at once when nothing was typed', async () => {
    signedInAs('admin', writer)
    server.use(http.get(`*/api/v1/admin/catalog/items/${bancoId}`, () => HttpResponse.json(item())))
    const { wrapper } = await openCatalog()
    const drawer = await openItem(wrapper)

    await drawer.trigger('keydown', { key: 'Escape' })

    expect(wrapper.find('[role=dialog]').exists()).toBe(false)
  })

  it.each([
    ['Escape', (drawer: Scope) => (drawer as ReturnType<VueWrapper['get']>).trigger('keydown', { key: 'Escape' })],
    ['Cancelar', (drawer: Scope) => button(drawer, 'Cancelar').trigger('click')],
  ])('asks before %s throws edits away, and keeps them on "Continuar editando"', async (_way, close) => {
    const { wrapper, drawer } = await dirtyDrawer()

    await close(drawer)
    await flushPromises()

    expect(wrapper.find('[role=dialog]').exists()).toBe(true)
    const question = drawer.get('[role=group][aria-label="Descartar as alterações"]')
    expect(question.text()).toContain('Descartar as alterações?')
    expect(document.activeElement?.textContent?.trim()).toBe('Continuar editando')
    await button(question, 'Continuar editando').trigger('click')
    expect(drawer.text()).not.toContain('Descartar as alterações?')
    expect((field(drawer, 'Nome').element as HTMLInputElement).value).toBe('Meu nome')
  })

  it('drops the edits on "Descartar"', async () => {
    const { wrapper, drawer } = await dirtyDrawer()

    await button(drawer, 'Cancelar').trigger('click')
    await flushPromises()
    await button(drawer, 'Descartar').trigger('click')

    expect(wrapper.find('[role=dialog]').exists()).toBe(false)
  })
})

describe('the item form checks', () => {
  it('asks for at least one production day', async () => {
    signedInAs('owner', writer)
    serve({ settings: settings({ productionEnabled: true }) })
    const { wrapper } = await openCatalog()
    await button(wrapper, 'Novo item').trigger('click')
    const drawer = wrapper.get('[role=dialog]')
    await field(drawer, 'Origem').setValue('in_house')

    await drawer.get('form').trigger('submit')
    expect(drawer.text()).toContain('Informe os dias de produção.')
    await field(drawer, 'Dias de produção').setValue('0')
    await drawer.get('form').trigger('submit')

    expect(drawer.text()).toContain('Use de 1 a 365 dias de produção.')
  })
})

describe('"No portal" while it is saved', () => {
  it('waits for the answer before the switch can be used again', async () => {
    signedInAs('admin', writer)
    const answer = gate()
    server.use(
      http.put(`*/api/v1/admin/catalog/items/${bancoId}/published`, async () => {
        await answer.opened
        return HttpResponse.json(item({ published: false, version: 4 }))
      }),
    )
    const { wrapper } = await openCatalog()
    const toggle = () => field(wrapper, 'Mostrar Banco de couro Scrambler na página da loja')

    await toggle().setValue(false)
    expect((toggle().element as HTMLInputElement).disabled).toBe(true)
    answer.open()
    await vi.waitFor(() => expect((toggle().element as HTMLInputElement).disabled).toBe(false))

    expect((toggle().element as HTMLInputElement).checked).toBe(false)
  })
})

describe('the Campos dialog keeps what is typed', () => {
  it('does not wipe another card when the fields are read again', async () => {
    signedInAs('admin', writer)
    const second: Attribute = { id: 'a2', name: 'Cor', position: 1, values: [{ id: 'b3', label: 'Preto', usageCount: 0 }] }
    serve({ attributes: [serveEm, second] })
    server.use(http.put(`*/api/v1/admin/catalog/attributes/${serveEm.id}`, () => HttpResponse.json(serveEm)))
    const { wrapper } = await openCatalog()
    await button(wrapper, 'Campos').trigger('click')
    const dialog = wrapper.get('[role=dialog]')
    const cards = dialog.findAll('form')
    const [first, other] = cards
    if (first === undefined || other === undefined) throw new Error('two cards expected')

    await field(other, 'Valor 1').setValue('Branco')
    await field(first, 'Nome do campo').setValue('Serve nas motos')
    await button(first, 'Salvar campo').trigger('click')
    await flushPromises()

    expect((field(other, 'Valor 1').element as HTMLInputElement).value).toBe('Branco')
  })

  it('moves focus to the removal question', async () => {
    signedInAs('admin', writer)
    const { wrapper } = await openCatalog()
    await button(wrapper, 'Campos').trigger('click')
    const dialog = wrapper.get('[role=dialog]')

    await button(dialog, 'Remover valor CB 500X').trigger('click')
    await flushPromises()

    expect(document.activeElement?.textContent?.trim()).toBe('Cancelar')
    expect(dialog.get('[role=group]').element.contains(document.activeElement)).toBe(true)
  })
})

describe('when the first read fails', () => {
  it('offers "Tentar de novo" and lists the items when it works', async () => {
    signedInAs('admin', writer)
    let reads = 0
    server.use(http.get('*/api/v1/admin/catalog/items', () => (++reads === 1 ? new HttpResponse(null, { status: 500 }) : HttpResponse.json({ items: [summary()] }))))
    const app = await openApp('/catalogo')
    await flushPromises()

    expect(app.wrapper.get('[role=alert]').text()).toBe('O servidor não respondeu. Tente de novo em instantes.')
    expect(app.wrapper.text()).not.toContain('Carregando o catálogo…')
    expect(app.wrapper.text()).not.toContain('Nenhum item no catálogo ainda.')

    await button(app.wrapper, 'Tentar de novo').trigger('click')
    await vi.waitFor(() => expect(app.wrapper.text()).toContain('Banco de couro Scrambler'))

    expect(app.wrapper.find('[role=alert]').exists()).toBe(false)
    expect(app.wrapper.findAll('button').map((b) => b.text())).not.toContain('Tentar de novo')
  })

  it('still offers it when only the categories failed', async () => {
    signedInAs('admin', writer)
    let reads = 0
    server.use(http.get('*/api/v1/admin/catalog/categories', () => (++reads === 1 ? new HttpResponse(null, { status: 500 }) : HttpResponse.json({ items: [bancos] }))))
    const app = await openApp('/catalogo')
    await vi.waitFor(() => expect(app.wrapper.text()).toContain('Banco de couro Scrambler'))

    await button(app.wrapper, 'Tentar de novo').trigger('click')
    await flushPromises()

    expect(app.wrapper.findAll('button').map((b) => b.text())).not.toContain('Tentar de novo')
  })
})

describe('archiving and restoring with edits not saved', () => {
  it('asks "Descartar as alterações?" before archiving, and archives only on "Descartar"', async () => {
    signedInAs('admin', writer)
    const archived: unknown[] = []
    server.use(
      http.get(`*/api/v1/admin/catalog/items/${bancoId}`, () => HttpResponse.json(item())),
      http.post(`*/api/v1/admin/catalog/items/${bancoId}/archive`, async ({ request }) => {
        archived.push(await request.json())
        return HttpResponse.json(item({ archived: true, published: false, version: 4 }))
      }),
    )
    const { wrapper } = await openCatalog()
    const drawer = await openItem(wrapper)
    await field(drawer, 'Nome').setValue('Outro nome')

    await button(drawer, 'Arquivar').trigger('click')
    await flushPromises()
    expect(drawer.text()).toContain('Descartar as alterações?')
    expect(archived).toEqual([])

    await button(drawer, 'Continuar editando').trigger('click')
    expect(drawer.text()).not.toContain('Descartar as alterações?')
    await button(drawer, 'Arquivar').trigger('click')
    await button(drawer, 'Descartar').trigger('click')
    await flushPromises()

    expect(archived).toEqual([{ version: 3 }])
  })

  it('asks before restoring too', async () => {
    signedInAs('admin', writer)
    const restored: unknown[] = []
    server.use(
      http.get(`*/api/v1/admin/catalog/items/${bancoId}`, () => HttpResponse.json(item({ archived: true, published: false }))),
      http.post(`*/api/v1/admin/catalog/items/${bancoId}/restore`, async ({ request }) => {
        restored.push(await request.json())
        return HttpResponse.json(item({ version: 4 }))
      }),
    )
    const { wrapper } = await openCatalog()
    const drawer = await openItem(wrapper)
    await field(drawer, 'Nome').setValue('Outro nome')

    await button(drawer, 'Restaurar').trigger('click')
    await flushPromises()
    expect(drawer.text()).toContain('Descartar as alterações?')
    expect(restored).toEqual([])
    await button(drawer, 'Descartar').trigger('click')
    await flushPromises()

    expect(restored).toEqual([{ version: 3 }])
  })
})
