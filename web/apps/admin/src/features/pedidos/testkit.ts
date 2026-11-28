import type { Item, ItemSummary, Kit, KitSummary, Quote } from '@encomendo/api'
import { http, HttpResponse } from 'msw'
import type { SetupServer } from 'msw/node'

import { brl, orderProblem } from '../../testing/orders'

// Fixtures and handlers the Pedidos tests share: a small catalog (a kit with
// a swappable item, loose items, a required option) and a quote whose figures
// are deliberately not the sum of anything, so a screen that added them up
// would show the difference.

const id = (n: number) => `0190f3c4-5e1a-7b2c-8d3e-${String(n).padStart(12, '0')}`

export const bancoId = id(501)
export const guidaoId = id(502)
export const guidaoAltoId = id(503)
export const farolId = id(504)
export const kitId = id(601)

function item(itemId: string, name: string, cents: number, overrides: Partial<Item> = {}): Item {
  return {
    id: itemId,
    name,
    description: '',
    categoryId: null,
    sourcingMode: 'to_order',
    price: brl(cents),
    productionDays: null,
    readyDays: 10,
    mainSupplier: null,
    backupSupplier: null,
    attributeValueIds: [],
    options: [],
    photos: [],
    published: true,
    archived: false,
    version: 1,
    ...overrides,
  }
}

export const corOption: Item['options'][number] = {
  id: id(701),
  name: 'Cor',
  required: true,
  choices: [
    { id: id(711), label: 'Preto', extraPrice: brl(0), extraDays: 0 },
    { id: id(712), label: 'Marrom', extraPrice: brl(5000), extraDays: 2 },
  ],
}

export const banco = item(bancoId, 'Banco de couro Scrambler', 45000, { options: [corOption] })
export const guidao = item(guidaoId, 'Guidão baixo', 30000)
export const guidaoAlto = item(guidaoAltoId, 'Guidão alto', 32000)
export const farol = item(farolId, 'Farol LED', 12000)
export const catalogItems: Item[] = [banco, guidao, guidaoAlto, farol]

export const kit: Kit = {
  id: kitId,
  name: 'Kit Scrambler',
  description: '',
  price: brl(70000),
  singleItemsTotal: brl(75000),
  discount: brl(5000),
  swapDiscount: 'keep',
  assemblyDays: null,
  safetyMarginDays: null,
  days: { slowestItem: 10, assembly: 3, safetyMargin: 2, total: 15 },
  items: [
    { itemId: bancoId, itemName: banco.name, quantity: 1, swappable: false, alternativeItemIds: [], unitPrice: brl(45000), readyDays: 10, archived: false },
    { itemId: guidaoId, itemName: guidao.name, quantity: 1, swappable: true, alternativeItemIds: [guidaoAltoId], unitPrice: brl(30000), readyDays: 10, archived: false },
  ],
  attributeValueIds: [],
  photos: [],
  published: true,
  archived: false,
  version: 1,
}

function summaryOf(source: Item): ItemSummary {
  return {
    id: source.id,
    name: source.name,
    categoryId: null,
    sourcingMode: source.sourcingMode,
    price: source.price,
    readyDays: source.readyDays,
    mainSupplierName: null,
    published: true,
    archived: false,
    cover: null,
    version: 1,
  }
}

const kitSummary: KitSummary = {
  id: kit.id,
  name: kit.name,
  itemCount: 2,
  price: kit.price,
  singleItemsTotal: kit.singleItemsTotal,
  totalDays: 15,
  published: true,
  archived: false,
  cover: null,
  version: 1,
}

/**
 * quoteFor is a quote whose figures are not sums of the lines: a total that
 * is not the items plus the freight, and a deposit that is not half of it.
 * The screen must show them as they are.
 */
export function quoteFor(overrides: Partial<Quote> = {}): Quote {
  return {
    lines: [],
    subtotal: brl(111100),
    freight: brl(2200),
    discount: brl(300),
    total: brl(123400),
    depositPercent: 40,
    depositDue: brl(55500),
    assemblyDays: 3,
    safetyMarginDays: 2,
    deliveryDate: '2026-12-20',
    arrivalLimit: '2026-12-17',
    ...overrides,
  }
}

/** serveCatalog answers the catalog reads of the order form: lists by name, and one kit or item by id. */
export function serveCatalog(server: SetupServer): void {
  server.use(
    http.get('*/api/v1/admin/catalog/kits', ({ request }) => {
      const q = new URL(request.url).searchParams.get('q')?.toLowerCase() ?? ''
      return HttpResponse.json({ items: [kitSummary].filter((found) => found.name.toLowerCase().includes(q)) })
    }),
    http.get('*/api/v1/admin/catalog/kits/:id', ({ params }) => (params.id === kit.id ? HttpResponse.json(kit) : orderProblem('not_found'))),
    http.get('*/api/v1/admin/catalog/items', ({ request }) => {
      const q = new URL(request.url).searchParams.get('q')?.toLowerCase() ?? ''
      return HttpResponse.json({ items: catalogItems.map(summaryOf).filter((found) => found.name.toLowerCase().includes(q)) })
    }),
    http.get('*/api/v1/admin/catalog/items/:id', ({ params }) => {
      const found = catalogItems.find((candidate) => candidate.id === params.id)
      return found === undefined ? orderProblem('not_found') : HttpResponse.json(found)
    }),
  )
}
