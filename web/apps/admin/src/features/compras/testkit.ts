import type {
  Notice,
  PurchaseLine,
  Receipt,
  ReceiptRequest,
  Supplier,
  SupplierItemUse,
  SupplierOrder,
  SupplierOrderSummary,
  TenantSettings,
  ToBuy,
  ToBuyItem,
  ToBuySupplier,
  ToReceive,
  ToReceiveLine,
} from '@encomendo/api'
import { http, HttpResponse } from 'msw'

import { brl, encomendaMontagem, settingsUsing } from '../../testing/orders'

/**
 * Fixtures for Compras: the answers of the purchasing endpoints, written out
 * as the API sends them. The owner's carry cost and paid price; the admin's
 * do not, as the API leaves them out.
 */

const id = (n: number) => `0190f3c4-5e1a-7b2c-8d3e-${String(n).padStart(12, '0')}`

export const couros = { id: id(7001), name: 'Couros Paulista', phone: '5511987654321' }
export const motoPecas = { id: id(7002), name: 'Moto Peças', phone: '5592991234567' }
export const orderIds = { first: id(8001), second: id(8002) }

/** wa.me links the API would answer; a test checks they are opened exactly as they are. */
export const links = {
  send: 'https://wa.me/5511987654321?text=Ol%C3%A1%2C+Couros+Paulista%21',
  notice: 'https://wa.me/5511987654321?text=n%C3%A3o+vou+mais+precisar',
  chat: 'https://wa.me/5511987654321',
}

/** toBuyItem is one pending item of the supplier's card; `owner` adds the unit cost only the owner gets. */
export function toBuyItem(n: number, overrides: Partial<ToBuyItem> = {}, owner = false): ToBuyItem {
  return {
    orderId: orderIds.first,
    orderNumber: 'P-0231',
    orderItemId: id(9000 + n),
    catalogItemId: id(9500 + n),
    name: 'Banco de couro CB 500X',
    quantity: 2,
    choices: [{ name: 'Cor', label: 'Preto' }],
    arrivalLimit: '2026-12-09',
    replacement: false,
    ...(owner ? { cost: brl(45000) } : {}),
    ...overrides,
  }
}

/** toBuyAnswer is "A comprar" with two suppliers: Couros Paulista with two items (the second a replacement) and Moto Peças with one. */
export function toBuyAnswer(owner = false): ToBuy {
  const couro: ToBuySupplier = {
    supplierId: couros.id,
    supplierName: couros.name,
    whatsappPhone: couros.phone,
    items: [
      toBuyItem(1, {}, owner),
      toBuyItem(2, { name: 'Capa de banco', quantity: 1, choices: [], orderId: orderIds.second, orderNumber: 'P-0232', arrivalLimit: '2026-12-15', replacement: true }, owner),
    ],
  }
  const moto: ToBuySupplier = {
    supplierId: motoPecas.id,
    supplierName: motoPecas.name,
    whatsappPhone: motoPecas.phone,
    items: [toBuyItem(3, { name: 'Pastilha de freio', quantity: 4, choices: [] }, owner)],
  }
  return { suppliers: [couro, moto], truncated: false }
}

/** purchaseLine is one line of a pedido de compra, sent; `owner` adds cost and paid price. */
export function purchaseLine(n: number, overrides: Partial<PurchaseLine> = {}, owner = false): PurchaseLine {
  return {
    id: id(6000 + n),
    orderId: orderIds.first,
    orderNumber: 'P-0231',
    orderItemId: id(9000 + n),
    catalogItemId: id(9500 + n),
    name: 'Banco de couro CB 500X',
    quantity: 2,
    choices: [{ name: 'Cor', label: 'Preto' }],
    status: 'sent',
    promisedDate: null,
    expectedDate: '2026-12-08',
    arrivalLimit: '2026-12-09',
    alerts: [],
    notice: null,
    checkNote: '',
    ...(owner ? { cost: brl(45000), paidCost: null } : {}),
    ...overrides,
  }
}

export const supplierOrderId = id(5001)

/** supplierOrderAnswer is C-0012 with two sent lines, version 1. */
export function supplierOrderAnswer(owner = false, overrides: Partial<SupplierOrder> = {}): SupplierOrder {
  return {
    id: supplierOrderId,
    number: 'C-0012',
    supplierId: couros.id,
    supplierName: couros.name,
    sentAt: '2026-11-27T15:00:00Z',
    promisedDate: null,
    received: 0,
    total: 2,
    state: 'sent',
    sentBy: { id: id(1), name: 'Ana Souza' },
    whatsappUrl: links.chat,
    lines: [purchaseLine(1, {}, owner), purchaseLine(2, { name: 'Capa de banco', quantity: 1, choices: [], orderId: orderIds.second, orderNumber: 'P-0232', arrivalLimit: '2026-12-15' }, owner)],
    canUndo: true,
    version: 1,
    ...overrides,
  }
}

/** summaryOf is the "Enviadas" row of a pedido de compra. */
export function summaryOf(order: SupplierOrder, overrides: Partial<SupplierOrderSummary> = {}): SupplierOrderSummary {
  return {
    id: order.id,
    number: order.number,
    supplierId: order.supplierId,
    supplierName: order.supplierName,
    sentAt: order.sentAt,
    promisedDate: order.promisedDate,
    received: order.received,
    total: order.total,
    state: order.state,
    ...overrides,
  }
}

/** noticeFor is a sent line that is no longer needed. */
export function noticeFor(n: number, overrides: Partial<Notice> = {}): Notice {
  return {
    lineId: id(6000 + n),
    supplierOrderId,
    number: 'C-0012',
    supplierName: couros.name,
    orderId: orderIds.first,
    orderNumber: 'P-0231',
    name: 'Banco de couro CB 500X',
    quantity: 2,
    reason: 'order_cancelled',
    whatsappUrl: links.notice,
    ...overrides,
  }
}

/** Backend is what the fake API answers; every field has a default. */
export interface Backend {
  toBuy?: ToBuy
  notices?: Notice[]
  orders?: SupplierOrderSummary[]
  settings?: TenantSettings
  suppliers?: Supplier[]
  supplierItems?: SupplierItemUse[]
  order?: SupplierOrder
}

/** Seen is what the screens asked, kept for the tests to check. */
export interface Seen {
  previews: { supplierId: string; orderItemIds: string[] }[]
  orderLists: URL[]
  supplierLists: URL[]
}

/** serve answers the reads of Compras from a backend and records the picks and queries it was asked for. */
export function serve(backend: Backend = {}) {
  const seen: Seen = { previews: [], orderLists: [], supplierLists: [] }
  const handlers = [
    http.get('*/api/v1/admin/settings', () => HttpResponse.json(backend.settings ?? settingsUsing(encomendaMontagem))),
    http.get('*/api/v1/admin/purchasing/to-buy', () => HttpResponse.json(backend.toBuy ?? toBuyAnswer())),
    http.get('*/api/v1/admin/purchasing/notices', () => HttpResponse.json({ items: backend.notices ?? [] })),
    http.post('*/api/v1/admin/purchasing/supplier-orders/preview', async ({ request }) => {
      const body = (await request.json()) as Seen['previews'][number]
      seen.previews.push(body)
      return HttpResponse.json({ text: `Pedido de compra C-0013 para ${body.orderItemIds.length} item(ns)` })
    }),
    http.get('*/api/v1/admin/purchasing/supplier-orders', ({ request }) => {
      seen.orderLists.push(new URL(request.url))
      return HttpResponse.json({ items: backend.orders ?? [] })
    }),
    http.get('*/api/v1/admin/purchasing/supplier-orders/:id', () => HttpResponse.json(backend.order ?? supplierOrderAnswer())),
    http.get('*/api/v1/admin/suppliers', ({ request }) => {
      seen.supplierLists.push(new URL(request.url))
      return HttpResponse.json({ items: backend.suppliers ?? [] })
    }),
    http.get('*/api/v1/admin/suppliers/:id/items', () => HttpResponse.json({ items: backend.supplierItems ?? [] })),
  ]
  return { handlers, seen }
}

/** receiveLine is one line to receive; with `counter` it has no `lineId`: an item bought at the counter, received by its `orderItemId`. */
export function receiveLine(n: number, overrides: Partial<ToReceiveLine> = {}, counter = false): ToReceiveLine {
  return {
    ...(counter ? {} : { lineId: id(6000 + n) }),
    orderId: orderIds.first,
    orderNumber: 'P-0231',
    orderItemId: id(9000 + n),
    name: 'Banco de couro CB 500X',
    quantity: 2,
    choices: [{ name: 'Cor', label: 'Preto' }],
    ...overrides,
  }
}

/** toReceiveAnswer is Receber itens: C-0012 from Couros Paulista with two lines, C-0013 from Moto Peças with one, and one item bought at the counter. */
export function toReceiveAnswer(): ToReceive {
  return {
    groups: [
      {
        supplierOrderId,
        number: 'C-0012',
        supplierName: couros.name,
        lines: [receiveLine(1), receiveLine(2, { name: 'Capa de banco', quantity: 1, choices: [], orderId: orderIds.second, orderNumber: 'P-0232' })],
      },
      { supplierOrderId: id(5002), number: 'C-0013', supplierName: motoPecas.name, lines: [receiveLine(3, { name: 'Pastilha de freio', quantity: 4, choices: [] })] },
    ],
    toBuy: [receiveLine(4, { name: 'Parafuso M8', quantity: 10, choices: [] }, true)],
    truncated: false,
  }
}

/** savedReceipt is the answer of a receipt that moved nothing but its entries. */
export function savedReceipt(overrides: Partial<Receipt> = {}): Receipt {
  return { id: id(4001), receivedAt: '2026-11-28T14:00:00Z', entries: [], completedOrders: [], backToBuy: [], ...overrides }
}

/** Receiving is what the fake API of Receber itens answers, and what it was asked. */
export interface Receiving {
  toReceive: ToReceive
  /** receipts are the saves it was sent, with their Idempotency-Key. */
  receipts: { body: ReceiptRequest; key: string | null }[]
  /** uploads are the content types asked for, in order. */
  uploads: string[]
  /** answers override a step by name: `upload`, `storage` or `receipt`. */
  answers: Partial<Record<'upload' | 'storage' | 'receipt', () => Response | Promise<Response>>>
  receipt: Receipt
  reads: number
}

/** newReceiving starts the fake API of Receber itens. */
export function newReceiving(overrides: Partial<Receiving> = {}): Receiving {
  return { toReceive: toReceiveAnswer(), receipts: [], uploads: [], answers: {}, receipt: savedReceipt(), reads: 0, ...overrides }
}

/** serveReceiving answers Receber itens: the list, the upload request, the storage `PUT` and the receipt. */
export function serveReceiving(state: Receiving) {
  return [
    http.get('*/api/v1/admin/purchasing/to-receive', () => {
      state.reads += 1
      return HttpResponse.json(state.toReceive)
    }),
    http.post('*/api/v1/admin/purchasing/receipt-photo-uploads', async ({ request }) => {
      const body = (await request.json()) as { contentType: string }
      state.uploads.push(body.contentType)
      const key = `t/tenant/receipts/photo-${state.uploads.length}`
      return (
        (await state.answers.upload?.()) ??
        HttpResponse.json({ key, uploadUrl: `http://storage.test/bucket/${key}`, method: 'PUT', headers: { 'Content-Type': body.contentType }, expiresAt: '2026-11-28T15:00:00Z' }, { status: 201 })
      )
    }),
    http.put('http://storage.test/bucket/*', async () => (await state.answers.storage?.()) ?? new HttpResponse(null, { status: 200 })),
    http.post('*/api/v1/admin/purchasing/receipts', async ({ request }) => {
      state.receipts.push({ body: (await request.json()) as ReceiptRequest, key: request.headers.get('Idempotency-Key') })
      return (await state.answers.receipt?.()) ?? HttpResponse.json(state.receipt, { status: 201 })
    }),
  ]
}
