import type { Category, ItemSummary, Kit, KitSummary, Permission, TenantSettings } from '@encomendo/api'
import { enableAutoUnmount, flushPromises, type VueWrapper } from '@vue/test-utils'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'

import { loadPage } from '../../pageLoad'
import { closeApps, openApp } from '../../testing/app'
import { authSession } from '../../testing/fixtures'
import { bancoId, bancos, brl, button, columnHeaders, field, group, guidoes, plain, problem, readOnly, serveEm, settings, summary, writer } from './testkit'

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

const premiumId = '0190f3c4-5e1a-7b2c-8d3e-000000000102'
const guidaoId = '0190f3c4-5e1a-7b2c-8d3e-000000000103'
const escapamentoId = '0190f3c4-5e1a-7b2c-8d3e-000000000104'
const pedalId = '0190f3c4-5e1a-7b2c-8d3e-000000000105'
const kitId = '0190f3c4-5e1a-7b2c-8d3e-000000000201'

const catalogItems: ItemSummary[] = [
  summary(),
  summary({ id: premiumId, name: 'Banco de couro Premium', price: brl(60000) }),
  summary({ id: guidaoId, name: 'Guidão alto', categoryId: guidoes.id, price: brl(30000) }),
  summary({ id: escapamentoId, name: 'Escapamento', categoryId: null, price: brl(90000) }),
  summary({ id: pedalId, name: 'Pedal antigo', archived: true, published: false }),
]

/** ownerCosts are what only the owner's kit responses carry (ADR 0025). */
const ownerCosts = { cost: brl(112000), margin: { cents: 48000, basisPoints: 3000 } }

function kitSummary(overrides: Partial<KitSummary> = {}): KitSummary {
  return {
    id: kitId,
    name: 'Kit Scrambler CB 500X',
    itemCount: 2,
    price: brl(150000),
    singleItemsTotal: brl(160000),
    totalDays: 20,
    published: true,
    archived: false,
    cover: null,
    version: 4,
    ...overrides,
  }
}

function kit(overrides: Partial<Kit> = {}): Kit {
  return {
    id: kitId,
    name: 'Kit Scrambler CB 500X',
    description: 'Visual scrambler',
    price: brl(150000),
    singleItemsTotal: brl(160000),
    discount: brl(10000),
    swapDiscount: 'keep',
    assemblyDays: null,
    safetyMarginDays: null,
    days: { slowestItem: 15, assembly: 3, safetyMargin: 2, total: 20 },
    items: [
      { itemId: bancoId, itemName: 'Banco de couro Scrambler', quantity: 1, swappable: true, alternativeItemIds: [premiumId], unitPrice: brl(45000), readyDays: 15, archived: false },
      { itemId: guidaoId, itemName: 'Guidão alto', quantity: 1, swappable: false, alternativeItemIds: [], unitPrice: brl(30000), readyDays: 7, archived: false },
    ],
    attributeValueIds: [],
    photos: [],
    published: true,
    archived: false,
    version: 4,
    ...overrides,
  }
}

interface Backend {
  kits?: KitSummary[]
  items?: ItemSummary[]
  categories?: Category[]
  settings?: TenantSettings
}

/** serve answers the reads every open of Catálogo makes, the Kits tab included. */
function serve(backend: Backend = {}) {
  server.use(
    http.get('*/readyz', () => HttpResponse.json({ status: 'ok' })),
    http.get('*/api/v1/admin/catalog/items', () => HttpResponse.json({ items: backend.items ?? catalogItems })),
    http.get('*/api/v1/admin/catalog/categories', () => HttpResponse.json({ items: backend.categories ?? [bancos, guidoes] })),
    http.get('*/api/v1/admin/catalog/attributes', () => HttpResponse.json({ items: [serveEm] })),
    http.get('*/api/v1/admin/suppliers', () => HttpResponse.json({ items: [] })),
    http.get('*/api/v1/admin/settings', () => HttpResponse.json(backend.settings ?? settings())),
    http.get('*/api/v1/admin/catalog/kits', () => HttpResponse.json({ items: backend.kits ?? [kitSummary()] })),
  )
}

const signedInAs = (role: 'owner' | 'admin' | 'staff' | 'viewer', permissions: Permission[]) =>
  server.use(http.post('*/auth/refresh', () => HttpResponse.json(authSession({ role, permissions }))))

/** openKits opens Catálogo and its Kits tab. */
async function openKits(name = 'Kit Scrambler CB 500X') {
  const app = await openApp('/catalogo')
  await button(app.wrapper, 'Kits').trigger('click')
  await vi.waitFor(() => expect(app.wrapper.text()).toContain(name))
  return app
}

/** openKit opens the editor on a kit, reading it from the API. */
async function openKit(wrapper: VueWrapper, name = 'Kit Scrambler CB 500X') {
  await button(wrapper, name).trigger('click')
  await vi.waitFor(() => expect(wrapper.find('[role=dialog]').exists()).toBe(true))
  await flushPromises()
  return wrapper.get('[role=dialog]')
}

const serveKit = (read: Kit) => server.use(http.get(`*/api/v1/admin/catalog/kits/${kitId}`, () => HttpResponse.json(read)))

beforeEach(() => serve())

describe('the Catálogo tabs', () => {
  it('moves between Itens and Kits with the arrow keys, Home and End', async () => {
    signedInAs('admin', writer)
    const { wrapper } = await openApp('/catalogo')
    const tablist = wrapper.get('[role=tablist]')
    const tabs = () => tablist.findAll('[role=tab]')
    const itens = tabs()[0]
    const kits = tabs()[1]

    expect(tabs().map((tab) => tab.text())).toEqual(['Itens', 'Kits'])
    expect(itens?.attributes('aria-selected')).toBe('true')
    // One tab is in the Tab order: the open one.
    expect(tabs().map((tab) => tab.attributes('tabindex'))).toEqual(['0', '-1'])

    await itens?.trigger('keydown', { key: 'ArrowRight' })
    await flushPromises()
    expect(tabs()[1]?.attributes('aria-selected')).toBe('true')
    expect(document.activeElement).toBe(kits?.element)
    expect(wrapper.get('[role=tabpanel]').attributes('id')).toBe('panel-kits')

    await tabs()[1]?.trigger('keydown', { key: 'ArrowRight' })
    await flushPromises()
    expect(tabs()[0]?.attributes('aria-selected')).toBe('true')
    expect(document.activeElement).toBe(itens?.element)

    await tabs()[0]?.trigger('keydown', { key: 'ArrowLeft' })
    await flushPromises()
    expect(tabs()[1]?.attributes('aria-selected')).toBe('true')

    await tabs()[1]?.trigger('keydown', { key: 'Home' })
    await flushPromises()
    expect(tabs()[0]?.attributes('aria-selected')).toBe('true')

    await tabs()[0]?.trigger('keydown', { key: 'End' })
    await flushPromises()
    expect(tabs()[1]?.attributes('aria-selected')).toBe('true')
  })
})

describe('the Kits table', () => {
  it('lists kits with item count, prices, sum, days and "No portal"', async () => {
    signedInAs('admin', writer)
    const { wrapper } = await openKits()

    expect(columnHeaders(wrapper)).toEqual(['Kit', 'Itens', 'Preço do kit', 'Soma avulsa', 'Prazo', 'No portal'])
    const row = wrapper.get('tbody tr')
    expect(row.findAll('td').map((cell) => plain(cell.text()))).toEqual(['2', 'R$ 1.500,00', 'R$ 1.600,00', '20 dias', ''])
    expect(row.get('[role=switch]').attributes('aria-label')).toBe('Mostrar Kit Scrambler CB 500X na página da loja')
  })

  it('shows Margem to the owner because the API sent it, and to nobody else', async () => {
    signedInAs('owner', writer)
    serve({ kits: [kitSummary({ ...ownerCosts })] })
    const owner = await openKits()

    expect(columnHeaders(owner.wrapper)).toContain('Margem')
    expect(plain(owner.wrapper.get('tbody').text())).toContain('R$ 480,00 (30%)')

    signedInAs('admin', writer)
    serve()
    const admin = await openKits()
    expect(columnHeaders(admin.wrapper)).not.toContain('Margem')
  })

  it('shows a dash for an owner whose kit has no cost to take a margin from', async () => {
    signedInAs('owner', writer)
    serve({ kits: [kitSummary({ cost: null, margin: null })] })
    const { wrapper } = await openKits()

    expect(wrapper.get('tbody').text()).toContain('—')
  })

  it('says when there are no kits, and offers "Novo kit" to who can write', async () => {
    signedInAs('admin', writer)
    serve({ kits: [] })
    const { wrapper } = await openKits('Nenhum kit ainda.')

    expect(wrapper.text()).toContain('Nenhum kit ainda.')
    expect(wrapper.findAll('button').filter((b) => b.text() === 'Novo kit')).toHaveLength(2)
  })

  it('marks an archived kit, and reads them with "Mostrar arquivados"', async () => {
    signedInAs('admin', writer)
    const asked: (string | null)[] = []
    server.use(
      http.get('*/api/v1/admin/catalog/kits', ({ request }) => {
        const flag = new URL(request.url).searchParams.get('includeArchived')
        asked.push(flag)
        return HttpResponse.json({ items: flag === null ? [kitSummary()] : [kitSummary(), kitSummary({ id: 'k2', name: 'Kit antigo', archived: true, published: false })] })
      }),
    )
    const { wrapper } = await openKits()

    await wrapper.get('input[type=checkbox]:not([role=switch])').setValue(true)
    await vi.waitFor(() => expect(wrapper.text()).toContain('Kit antigo'))

    expect(asked).toEqual([null, 'true'])
    expect(wrapper.get('tbody').text()).toContain('Arquivado')
  })

  it('loads more with the cursor', async () => {
    signedInAs('admin', writer)
    const cursors: (string | null)[] = []
    server.use(
      http.get('*/api/v1/admin/catalog/kits', ({ request }) => {
        const cursor = new URL(request.url).searchParams.get('cursor')
        cursors.push(cursor)
        return cursor === null ? HttpResponse.json({ items: [kitSummary()], nextCursor: 'c2' }) : HttpResponse.json({ items: [kitSummary({ id: 'k2', name: 'Kit Trail' })] })
      }),
    )
    const { wrapper } = await openKits()

    await button(wrapper, 'Carregar mais').trigger('click')
    await vi.waitFor(() => expect(wrapper.text()).toContain('Kit Trail'))

    expect(cursors).toEqual([null, 'c2'])
  })

  it('switches "No portal" with the kit version', async () => {
    signedInAs('admin', writer)
    const bodies: unknown[] = []
    server.use(
      http.put(`*/api/v1/admin/catalog/kits/${kitId}/published`, async ({ request }) => {
        bodies.push(await request.json())
        return HttpResponse.json(kit({ published: false, version: 5 }))
      }),
    )
    const { wrapper } = await openKits()

    const toggle = field(wrapper, 'Mostrar Kit Scrambler CB 500X na página da loja')
    expect((toggle.element as HTMLInputElement).checked).toBe(true)
    await toggle.setValue(false)
    await flushPromises()

    expect(bodies).toEqual([{ published: false, version: 4 }])
    expect((field(wrapper, 'Mostrar Kit Scrambler CB 500X na página da loja').element as HTMLInputElement).checked).toBe(false)
  })
})

describe('a role that cannot write', () => {
  it('sees no editing control, in the table or in the kit', async () => {
    signedInAs('viewer', readOnly)
    serveKit(kit())
    const { wrapper } = await openKits()

    expect(wrapper.findAll('button').map((b) => b.text())).not.toContain('Novo kit')
    expect(wrapper.get('[role=switch]').attributes('disabled')).toBeDefined()

    const drawer = await openKit(wrapper)

    expect(drawer.get('h2').text()).toBe('Kit')
    expect(drawer.get('fieldset').attributes('disabled')).toBeDefined()
    const names = drawer.findAll('button').map((b) => b.text())
    expect(names).toContain('Fechar')
    for (const hidden of ['Salvar', 'Cancelar', 'Arquivar', 'Restaurar', 'Adicionar item', 'Remover do kit', 'Adicionar fotos']) {
      expect(names).not.toContain(hidden)
    }
    expect(drawer.text()).not.toContain('Item a adicionar')
  })
})

describe('the kit editor', () => {
  it('titles a new kit and an edited one', async () => {
    signedInAs('admin', writer)
    serveKit(kit())
    const { wrapper } = await openKits()

    await button(wrapper, 'Novo kit').trigger('click')
    expect(wrapper.get('[role=dialog] h2').text()).toBe('Novo kit')
    expect(wrapper.get('[role=dialog]').text()).toContain('Os totais e o prazo aparecem depois de salvar.')
    expect(wrapper.get('[role=dialog]').text()).toContain('Salve o kit para adicionar fotos.')
    await button(wrapper.get('[role=dialog]'), 'Cancelar').trigger('click')
    expect(wrapper.find('[role=dialog]').exists()).toBe(false)

    const drawer = await openKit(wrapper)
    expect(drawer.get('h2').text()).toBe('Editar kit')
    expect((field(drawer, 'Nome').element as HTMLInputElement).value).toBe('Kit Scrambler CB 500X')
    expect(drawer.findAll('button').map((b) => b.text())).toEqual(expect.arrayContaining(['Salvar', 'Cancelar', 'Arquivar']))
  })

  it('creates a kit with two items, one swappable with alternatives', async () => {
    signedInAs('admin', writer)
    serve({ kits: [] })
    const bodies: unknown[] = []
    server.use(
      http.post('*/api/v1/admin/catalog/kits', async ({ request }) => {
        bodies.push(await request.json())
        return HttpResponse.json(kit(), { status: 201 })
      }),
    )
    const { wrapper } = await openKits('Nenhum kit ainda.')
    await button(wrapper, 'Novo kit').trigger('click')
    const drawer = wrapper.get('[role=dialog]')

    await field(drawer, 'Nome').setValue(' Kit Scrambler CB 500X ')
    await field(drawer, 'Descrição').setValue('Visual scrambler')
    await field(drawer, 'Preço do kit').setValue('1.500,00')
    await field(drawer, 'Item a adicionar').setValue(bancoId)
    await button(drawer, 'Adicionar item').trigger('click')
    await field(drawer, 'Item a adicionar').setValue(guidaoId)
    await button(drawer, 'Adicionar item').trigger('click')
    await field(group(drawer, 'Guidão alto'), 'Quantidade').setValue('2')
    const banco = group(drawer, 'Banco de couro Scrambler')
    await field(banco, 'Cliente pode trocar').setValue(true)
    await field(group(banco, 'Pode trocar por'), 'Banco de couro Premium').setValue(true)
    await field(group(banco, 'Pode trocar por'), 'Escapamento').setValue(true)
    await drawer.get('form').trigger('submit')
    await flushPromises()

    expect(bodies).toEqual([
      {
        name: 'Kit Scrambler CB 500X',
        description: 'Visual scrambler',
        priceCents: 150000,
        swapDiscount: 'keep',
        assemblyDays: null,
        safetyMarginDays: null,
        items: [
          { itemId: bancoId, quantity: 1, swappable: true, alternativeItemIds: [premiumId, escapamentoId] },
          { itemId: guidaoId, quantity: 2, swappable: false, alternativeItemIds: [] },
        ],
        attributeValueIds: [],
        published: false,
      },
    ])
  })

  it('offers only non-archived items that are not in the kit yet, and takes an item out', async () => {
    signedInAs('admin', writer)
    serve({ kits: [] })
    const { wrapper } = await openKits('Nenhum kit ainda.')
    await button(wrapper, 'Novo kit').trigger('click')
    const drawer = wrapper.get('[role=dialog]')
    const picker = () => field(drawer, 'Item a adicionar').findAll('option').map((o) => o.text())

    expect(picker()).toEqual(['Escolha o item', 'Banco de couro Scrambler', 'Banco de couro Premium', 'Guidão alto', 'Escapamento'])

    await field(drawer, 'Item a adicionar').setValue(guidaoId)
    await button(drawer, 'Adicionar item').trigger('click')
    expect(picker()).not.toContain('Guidão alto')

    await button(drawer, 'Remover Guidão alto do kit').trigger('click')
    expect(picker()).toContain('Guidão alto')
    expect(drawer.findAll('fieldset').some((fieldset) => fieldset.find('legend')?.text() === 'Guidão alto')).toBe(false)
  })

  it('suggests the items of the same category first, and lists the rest after them', async () => {
    signedInAs('admin', writer)
    serveKit(kit())
    const { wrapper } = await openKits()
    const drawer = await openKit(wrapper)

    const swap = group(group(drawer, 'Banco de couro Scrambler'), 'Pode trocar por')
    const legends = swap.findAll('legend').map((legend) => legend.text())

    expect(legends).toEqual(['Pode trocar por', 'Sugestões da categoria Bancos', 'Outros itens'])
    expect(group(swap, 'Sugestões da categoria Bancos').findAll('label').map((l) => l.text())).toEqual(['Banco de couro Premium'])
    expect(group(swap, 'Outros itens').findAll('label').map((l) => l.text())).toEqual(['Guidão alto', 'Escapamento'])
    // The item itself is never its own alternative, and an archived item is not offered.
    expect(swap.text()).not.toContain('Pedal antigo')
    expect(swap.findAll('label').map((l) => l.text())).not.toContain('Banco de couro Scrambler')
    expect((field(swap, 'Banco de couro Premium').element as HTMLInputElement).checked).toBe(true)
  })

  it('lists one flat group when the item has no category to suggest from', async () => {
    signedInAs('admin', writer)
    serveKit(
      kit({
        items: [{ itemId: escapamentoId, itemName: 'Escapamento', quantity: 1, swappable: true, alternativeItemIds: [], unitPrice: brl(90000), readyDays: 5, archived: false }],
      }),
    )
    const { wrapper } = await openKits()
    const drawer = await openKit(wrapper)

    const swap = group(group(drawer, 'Escapamento'), 'Pode trocar por')

    expect(swap.findAll('legend').map((legend) => legend.text())).toEqual(['Pode trocar por'])
    expect(swap.findAll('label').map((l) => l.text())).toEqual(['Banco de couro Scrambler', 'Banco de couro Premium', 'Guidão alto'])
  })

  it('saves both swap modes, and starts a new kit on "Manter o desconto do kit"', async () => {
    signedInAs('admin', writer)
    serveKit(kit())
    const modes: unknown[] = []
    server.use(
      http.put(`*/api/v1/admin/catalog/kits/${kitId}`, async ({ request }) => {
        modes.push(((await request.json()) as { swapDiscount: string }).swapDiscount)
        return HttpResponse.json(kit())
      }),
    )
    const { wrapper } = await openKits()
    const drawer = await openKit(wrapper)
    const swap = group(drawer, 'Ao trocar um item')

    expect((field(swap, 'Manter o desconto do kit').element as HTMLInputElement).checked).toBe(true)
    expect(swap.text()).toContain('Preço do kit − item retirado + item novo, pelos preços avulsos.')
    expect(swap.text()).toContain('O kit trocado custa a soma dos preços avulsos dos seus itens.')

    await field(swap, 'Cobrar a soma dos itens avulsos').setValue()
    await drawer.get('form').trigger('submit')
    await flushPromises()
    await field(group(drawer, 'Ao trocar um item'), 'Manter o desconto do kit').setValue()
    await drawer.get('form').trigger('submit')
    await flushPromises()

    expect(modes).toEqual(['lose', 'keep'])
  })

  it('sends null for days left blank, and the number for days the kit sets', async () => {
    signedInAs('admin', writer)
    serveKit(kit({ assemblyDays: 4, safetyMarginDays: null }))
    const bodies: { assemblyDays: unknown; safetyMarginDays: unknown }[] = []
    server.use(
      http.put(`*/api/v1/admin/catalog/kits/${kitId}`, async ({ request }) => {
        bodies.push((await request.json()) as { assemblyDays: unknown; safetyMarginDays: unknown })
        return HttpResponse.json(kit())
      }),
    )
    const { wrapper } = await openKits()
    const drawer = await openKit(wrapper)

    // The store's defaults are the hints, and what is blank follows them.
    expect((field(drawer, 'Dias de Montagem').element as HTMLInputElement).value).toBe('4')
    expect((field(drawer, 'Margem de segurança (dias)').element as HTMLInputElement).value).toBe('')
    expect(drawer.text()).toContain('Padrão da loja: 3 dias')
    expect(drawer.text()).toContain('Padrão da loja: 2 dias')
    await drawer.get('form').trigger('submit')
    await flushPromises()
    await field(drawer, 'Dias de Montagem').setValue('')
    await field(drawer, 'Margem de segurança (dias)').setValue('0')
    await drawer.get('form').trigger('submit')
    await flushPromises()

    expect(bodies).toEqual([
      { ...bodies[0], assemblyDays: 4, safetyMarginDays: null },
      { ...bodies[1], assemblyDays: null, safetyMarginDays: 0 },
    ])
  })

  it('takes the niche word from the default word set, and writes one day in the singular', async () => {
    signedInAs('admin', writer)
    serve({ settings: settings({ assemblyDays: 1 }) })
    serveKit(kit())
    const { wrapper } = await openKits()
    const drawer = await openKit(wrapper)

    expect(drawer.text()).toContain('Dias de Montagem')
    expect(drawer.text()).toContain('Padrão da loja: 1 dia')
  })

  it('names the fields to fix before calling the API', async () => {
    signedInAs('admin', writer)
    serve({ kits: [] })
    const { wrapper } = await openKits('Nenhum kit ainda.')
    await button(wrapper, 'Novo kit').trigger('click')
    const drawer = wrapper.get('[role=dialog]')

    await drawer.get('form').trigger('submit')
    await flushPromises()

    expect(drawer.get('[role=alert]').text()).toBe('Confira os campos destacados.')
    for (const message of ['Informe o nome do kit.', 'Informe o preço do kit.', 'Adicione ao menos um item.']) expect(drawer.text()).toContain(message)

    await field(drawer, 'Item a adicionar').setValue(bancoId)
    await button(drawer, 'Adicionar item').trigger('click')
    await field(drawer, 'Cliente pode trocar').setValue(true)
    await field(drawer, 'Quantidade').setValue('100')
    await field(drawer, 'Dias de Montagem').setValue('61')
    await drawer.get('form').trigger('submit')
    await flushPromises()

    expect(drawer.text()).toContain('Escolha ao menos um item para a troca.')
    expect(drawer.text()).toContain('Use uma quantidade de 1 a 99.')
    expect(drawer.text()).toContain('Use de 0 a 60 dias.')
  })

  it('shows the field messages the API sends next to their fields', async () => {
    signedInAs('admin', writer)
    serveKit(kit())
    server.use(
      http.put(`*/api/v1/admin/catalog/kits/${kitId}`, () =>
        problem(400, 'invalid', 'Dados inválidos', {
          errors: [
            { field: 'priceCents', message: 'Informe o preço do kit.' },
            { field: 'items[1].quantity', message: 'Use uma quantidade de 1 a 99.' },
            { field: 'items[0].alternativeItemIds', message: 'Escolha ao menos um item para a troca.' },
          ],
        }),
      ),
    )
    const { wrapper } = await openKits()
    const drawer = await openKit(wrapper)

    await drawer.get('form').trigger('submit')
    await flushPromises()

    expect(drawer.get('[role=alert]').text()).toBe('Confira os campos destacados.')
    expect(field(drawer, 'Preço do kit').attributes('aria-invalid')).toBe('true')
    expect(group(drawer, 'Guidão alto').text()).toContain('Use uma quantidade de 1 a 99.')
    expect(group(drawer, 'Banco de couro Scrambler').text()).toContain('Escolha ao menos um item para a troca.')
  })
})

describe('the totals', () => {
  it('shows what the API sent, exactly, and never works them out', async () => {
    signedInAs('admin', writer)
    // Figures that do not add up on purpose: whatever the server says is what shows.
    serveKit(
      kit({
        price: brl(150000),
        singleItemsTotal: brl(123456),
        discount: brl(7777),
        days: { slowestItem: 9, assembly: 4, safetyMargin: 1, total: 41 },
      }),
    )
    const { wrapper } = await openKits()
    const drawer = await openKit(wrapper)

    const totals = plain(drawer.get('[aria-label="Totais do kit"]').text())

    expect(totals).toContain('Soma dos itens avulsos: R$ 1.234,56')
    expect(totals).toContain('Desconto do kit: R$ 77,77')
    expect(totals).toContain('Prazo calculado: 41 dias (item mais lento 9 + Montagem 4 + margem de segurança 1)')
    expect(totals).not.toContain('Margem:')
  })

  it('shows Margem to the owner, as sent, and "—" when it has no cost', async () => {
    signedInAs('owner', writer)
    serveKit(kit({ ...ownerCosts }))
    const { wrapper } = await openKits()
    const drawer = await openKit(wrapper)

    expect(plain(drawer.get('[aria-label="Totais do kit"]').text())).toContain('Margem: R$ 480,00 (30%)')

    await button(drawer, 'Cancelar').trigger('click')
    serveKit(kit({ cost: null, margin: null }))
    const again = await openKit(wrapper)
    expect(plain(again.get('[aria-label="Totais do kit"]').text())).toContain('Margem: —')
  })

  it('keeps the last saved totals while the form is edited, and says they update after saving', async () => {
    signedInAs('admin', writer)
    serveKit(kit())
    const { wrapper } = await openKits()
    const drawer = await openKit(wrapper)
    const totals = () => plain(drawer.get('[aria-label="Totais do kit"]').text())
    const before = totals()
    expect(before).not.toContain('últim')

    await field(group(drawer, 'Guidão alto'), 'Quantidade').setValue('5')
    await field(drawer, 'Preço do kit').setValue('99,00')

    expect(totals()).toContain('Os totais e o prazo mostram o último salvamento. Eles se atualizam depois de salvar.')
    expect(totals()).toContain('Soma dos itens avulsos: R$ 1.600,00')
    expect(totals()).toContain('Prazo calculado: 20 dias')
  })

  it('stays open after saving, with the totals the API answered and "Kit salvo."', async () => {
    signedInAs('admin', writer)
    serveKit(kit())
    server.use(
      http.put(`*/api/v1/admin/catalog/kits/${kitId}`, () =>
        HttpResponse.json(kit({ version: 5, singleItemsTotal: brl(200000), discount: brl(50000), days: { slowestItem: 15, assembly: 3, safetyMargin: 2, total: 20 } })),
      ),
    )
    const { wrapper } = await openKits()
    const drawer = await openKit(wrapper)
    await field(drawer, 'Nome').setValue('Kit renomeado')

    await drawer.get('form').trigger('submit')
    await flushPromises()

    expect(wrapper.find('[role=dialog]').exists()).toBe(true)
    expect(drawer.get('[role=status]').text()).toBe('Kit salvo.')
    expect(document.activeElement).toBe(drawer.get('[role=status]').element)
    expect(plain(drawer.get('[aria-label="Totais do kit"]').text())).toContain('Soma dos itens avulsos: R$ 2.000,00')
    expect(drawer.get('[aria-label="Totais do kit"]').text()).not.toContain('últim')
  })

  it('warns, without blocking, when the API says the kit costs more than its items', async () => {
    signedInAs('admin', writer)
    serveKit(kit({ price: brl(170000), singleItemsTotal: brl(160000), discount: brl(-10000) }))
    const { wrapper } = await openKits()
    const drawer = await openKit(wrapper)

    expect(drawer.get('[role=status]').text()).toBe('O preço do kit está acima da soma avulsa.')
    expect(button(drawer, 'Salvar').attributes('disabled')).toBeUndefined()
  })

  it('does not warn for a kit with a discount, or one at the sum', async () => {
    signedInAs('admin', writer)
    serveKit(kit({ discount: brl(0) }))
    const { wrapper } = await openKits()
    const drawer = await openKit(wrapper)

    expect(drawer.text()).not.toContain('acima da soma avulsa')
  })
})

describe('archived items', () => {
  it('marks a row whose item was archived after the kit was saved', async () => {
    signedInAs('admin', writer)
    serveKit(
      kit({
        items: [
          { itemId: pedalId, itemName: 'Pedal antigo', quantity: 1, swappable: false, alternativeItemIds: [], unitPrice: brl(1000), readyDays: 2, archived: true },
          { itemId: guidaoId, itemName: 'Guidão alto', quantity: 1, swappable: false, alternativeItemIds: [], unitPrice: brl(30000), readyDays: 7, archived: false },
        ],
      }),
    )
    const { wrapper } = await openKits()
    const drawer = await openKit(wrapper)

    expect(group(drawer, 'Pedal antigo').text()).toContain('Arquivado')
    expect(group(drawer, 'Guidão alto').text()).not.toContain('Arquivado')
  })

  it('refuses to save and names the archived item', async () => {
    signedInAs('admin', writer)
    serveKit(kit())
    server.use(
      http.put(`*/api/v1/admin/catalog/kits/${kitId}`, () =>
        problem(409, 'item_archived', 'Item arquivado', { references: [{ id: pedalId, name: 'Pedal antigo' }] }),
      ),
    )
    const { wrapper } = await openKits()
    const drawer = await openKit(wrapper)

    await drawer.get('form').trigger('submit')
    await flushPromises()

    expect(drawer.get('[role=alert]').text()).toBe('Pedal antigo está arquivado e não pode entrar no kit.')
    expect(wrapper.find('[role=dialog]').exists()).toBe(true)
    expect(drawer.text()).not.toContain('Recarregar')
  })

  it('names every archived item when there are several', async () => {
    signedInAs('admin', writer)
    serveKit(kit())
    server.use(
      http.put(`*/api/v1/admin/catalog/kits/${kitId}`, () =>
        problem(409, 'item_archived', 'Item arquivado', {
          references: [
            { id: pedalId, name: 'Pedal antigo' },
            { id: escapamentoId, name: 'Escapamento' },
          ],
        }),
      ),
    )
    const { wrapper } = await openKits()
    const drawer = await openKit(wrapper)

    await drawer.get('form').trigger('submit')
    await flushPromises()

    expect(drawer.get('[role=alert]').text()).toBe('Pedal antigo está arquivado e não pode entrar no kit. Escapamento está arquivado e não pode entrar no kit.')
  })
})

describe('archiving, restoring and conflicts', () => {
  it('archives a kit and says so', async () => {
    signedInAs('admin', writer)
    serveKit(kit())
    const bodies: unknown[] = []
    server.use(
      http.post(`*/api/v1/admin/catalog/kits/${kitId}/archive`, async ({ request }) => {
        bodies.push(await request.json())
        return HttpResponse.json(kit({ archived: true, published: false, version: 5 }))
      }),
    )
    const { wrapper } = await openKits()
    const drawer = await openKit(wrapper)

    await button(drawer, 'Arquivar').trigger('click')
    await flushPromises()

    expect(bodies).toEqual([{ version: 4 }])
    expect(wrapper.find('[role=dialog]').exists()).toBe(false)
    expect(wrapper.get('[role=status]').text()).toBe('Kit Scrambler CB 500X foi arquivado.')
  })

  it('restores an archived kit, and the refusal for an archived item says which', async () => {
    signedInAs('admin', writer)
    serveKit(kit({ archived: true, published: false }))
    let answer: 'refuse' | 'ok' = 'refuse'
    server.use(
      http.post(`*/api/v1/admin/catalog/kits/${kitId}/restore`, () =>
        answer === 'refuse' ? problem(409, 'item_archived', 'Item arquivado', { references: [{ id: pedalId, name: 'Pedal antigo' }] }) : HttpResponse.json(kit({ version: 6 })),
      ),
    )
    const { wrapper } = await openKits()
    const drawer = await openKit(wrapper)

    expect(drawer.findAll('button').map((b) => b.text())).not.toContain('Arquivar')
    await button(drawer, 'Restaurar').trigger('click')
    await flushPromises()
    expect(drawer.get('[role=alert]').text()).toBe('Pedal antigo está arquivado e não pode entrar no kit.')

    answer = 'ok'
    await button(drawer, 'Restaurar').trigger('click')
    await flushPromises()
    expect(wrapper.get('[role=status]').text()).toBe('Kit Scrambler CB 500X voltou ao catálogo.')
  })

  it('says someone else saved first, and reads the kit again on "Recarregar"', async () => {
    signedInAs('admin', writer)
    serveKit(kit())
    server.use(http.put(`*/api/v1/admin/catalog/kits/${kitId}`, () => problem(409, 'version_conflict', 'Versão desatualizada')))
    const { wrapper } = await openKits()
    const drawer = await openKit(wrapper)
    await field(drawer, 'Nome').setValue('Minha edição')

    await drawer.get('form').trigger('submit')
    await flushPromises()

    expect(drawer.get('[role=alert]').text()).toContain('Outra pessoa alterou este kit. Recarregue para ver a versão atual.')

    serveKit(kit({ name: 'Kit da outra pessoa', version: 5 }))
    await button(drawer, 'Recarregar').trigger('click')
    await flushPromises()

    expect((field(wrapper.get('[role=dialog]'), 'Nome').element as HTMLInputElement).value).toBe('Kit da outra pessoa')
    expect(wrapper.get('[role=dialog]').text()).not.toContain('Recarregar')
  })

  it('sends the version that was read', async () => {
    signedInAs('admin', writer)
    serveKit(kit({ version: 7 }))
    const versions: unknown[] = []
    server.use(
      http.put(`*/api/v1/admin/catalog/kits/${kitId}`, async ({ request }) => {
        versions.push(((await request.json()) as { version: number }).version)
        return HttpResponse.json(kit({ version: 8 }))
      }),
    )
    const { wrapper } = await openKits()
    const drawer = await openKit(wrapper)

    await drawer.get('form').trigger('submit')
    await flushPromises()

    expect(versions).toEqual([7])
  })
})

describe('unsaved edits in the kit editor', () => {
  it('asks before dropping them, and closes at once when nothing was typed', async () => {
    signedInAs('admin', writer)
    serveKit(kit())
    const { wrapper } = await openKits()
    let drawer = await openKit(wrapper)

    await button(drawer, 'Cancelar').trigger('click')
    expect(wrapper.find('[role=dialog]').exists()).toBe(false)

    drawer = await openKit(wrapper)
    await field(drawer, 'Nome').setValue('Outro nome')
    await button(drawer, 'Cancelar').trigger('click')
    await flushPromises()

    expect(drawer.text()).toContain('Descartar as alterações?')
    expect(document.activeElement?.textContent?.trim()).toBe('Continuar editando')
    await button(drawer, 'Descartar').trigger('click')
    expect(wrapper.find('[role=dialog]').exists()).toBe(false)
  })
})

describe('kit photos', () => {
  it('asks a new kit to be saved first, and uploads to the kit once it is', async () => {
    signedInAs('admin', writer)
    serveKit(kit())
    const uploads: unknown[] = []
    server.use(
      http.post('*/api/v1/admin/catalog/photo-uploads', async ({ request }) => {
        uploads.push(await request.json())
        return HttpResponse.json({ key: `t/x/kits/${kitId}/p1`, uploadUrl: 'http://storage.test/put', method: 'PUT', headers: {}, expiresAt: '2026-12-01T00:00:00Z' })
      }),
      http.put('http://storage.test/put', () => new HttpResponse(null, { status: 200 })),
      http.post(`*/api/v1/admin/catalog/kits/${kitId}/photos`, () =>
        HttpResponse.json({ id: 'p1', url: 'http://storage.test/p1', expiresAt: '2026-12-01T00:00:00Z', position: 0 }, { status: 201 }),
      ),
    )
    const { wrapper } = await openKits()
    const drawer = await openKit(wrapper)
    const input = drawer.get('input[type=file]')
    const file = new File(['x'], 'kit.jpg', { type: 'image/jpeg' })
    Object.defineProperty(input.element, 'files', { value: [file], configurable: true })

    await input.trigger('change')
    await vi.waitFor(() => expect(drawer.find('img').exists()).toBe(true))

    expect(uploads).toEqual([{ ownerType: 'kit', ownerId: kitId, contentType: 'image/jpeg' }])
    expect(drawer.get('img').attributes('alt')).toBe('Foto 1')
  })

  it('says a kit holds up to 8 photos', async () => {
    signedInAs('admin', writer)
    serveKit(kit())
    server.use(http.post('*/api/v1/admin/catalog/photo-uploads', () => problem(409, 'photo_limit', 'Limite de fotos')))
    const { wrapper } = await openKits()
    const drawer = await openKit(wrapper)
    const input = drawer.get('input[type=file]')
    Object.defineProperty(input.element, 'files', { value: [new File(['x'], 'kit.jpg', { type: 'image/jpeg' })], configurable: true })

    await input.trigger('change')
    await flushPromises()

    expect(drawer.get('[role=alert]').text()).toBe('Cada kit pode ter até 8 fotos.')
  })
})
