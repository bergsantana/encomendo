import type { HttpClient } from './http'
import { idempotencyHeaders } from './idempotency'
import { type PageRequest, queryString } from './query'
import type { components } from './types/schema'

type Schemas = components['schemas']

/** ToBuy is Compras, "A comprar": the bought items still to buy, by supplier. */
export type ToBuy = Schemas['ToBuy']
/** ToBuySupplier is one supplier's card in "A comprar". */
export type ToBuySupplier = Schemas['ToBuySupplier']
/** ToBuyItem is one pending item. Its `cost` is the owner's only. */
export type ToBuyItem = Schemas['ToBuyItem']
/** PurchaseChoice is an option choice of a bought item ("Cor: Preto"). */
export type PurchaseChoice = Schemas['PurchaseChoice']
/** SupplierOrderRequest picks a supplier's items for a pedido de compra; the same body previews and sends. */
export type SupplierOrderRequest = Schemas['SupplierOrderRequest']
/** SupplierMessagePreview is the text a pedido de compra would send, for "Ver mensagem". */
export type SupplierMessagePreview = Schemas['SupplierMessagePreview']
/** SentSupplierOrder is the new pedido de compra with the wa.me link and the text the app opens and copies. */
export type SentSupplierOrder = Schemas['SentSupplierOrder']
/** SupplierOrderSummary is one row of "Enviadas". */
export type SupplierOrderSummary = Schemas['SupplierOrderSummary']
/** SupplierOrderPage is one page of "Enviadas", newest first. */
export type SupplierOrderPage = Schemas['SupplierOrderPage']
/** SupplierOrder is one pedido de compra with its lines. `cost` and `paidCost` are the owner's only. */
export type SupplierOrder = Schemas['SupplierOrder']
/** SupplierOrderState is the situation of a pedido de compra: sent, in transit, received or undone. */
export type SupplierOrderState = Schemas['SupplierOrderState']
/** PurchaseLine is one line of a pedido de compra. */
export type PurchaseLine = Schemas['PurchaseLine']
/** PurchaseLineStatus is where a line is: sent, in transit, received, defective or cancelled. */
export type PurchaseLineStatus = Schemas['PurchaseLineStatus']
/** PromisedDateRequest is the supplier's promised date, with per-line overrides, and the version read. */
export type PromisedDateRequest = Schemas['PromisedDateRequest']
/** PaidPricesRequest is the unit price paid per line (the owner's), and the version read. */
export type PaidPricesRequest = Schemas['PaidPricesRequest']
/** SupplierOrderVersionRequest is the body of the calls that only need the pedido de compra's `version`. */
export type SupplierOrderVersionRequest = Schemas['VersionRequest']
/** Notice is a sent line that is no longer needed ("Avisar o fornecedor"), with its wa.me link. */
export type Notice = Schemas['Notice']
/** NoticeList is the notices, oldest send first. */
export type NoticeList = Schemas['NoticeList']
/** NoticeReason says why a sent line is no longer needed. */
export type NoticeReason = Schemas['NoticeReason']
/** OrderPurchases is what purchasing knows of one order's bought items. */
export type OrderPurchases = Schemas['OrderPurchases']
/** OrderPurchase is one bought item of an order: still to buy, or with its open or last line. */
export type OrderPurchase = Schemas['OrderPurchase']
/** PurchasingToday is Hoje's purchasing entries. */
export type PurchasingToday = Schemas['PurchasingToday']
/** PurchasingNeed is one purchasing entry of "Precisa de você". */
export type PurchasingNeed = Schemas['PurchasingNeed']

/** SupplierOrderFilter narrows "Enviadas": `open` (the default) is sent or in transit, `all` is every pedido de compra. */
export interface SupplierOrderFilter extends PageRequest {
  state?: 'open' | 'all'
}

// Compras needs `purchase:write` (owner, admin). `cost` and `paidCost` reach
// the owner only; for every other role the fields are simply absent.

const purchasing = '/api/v1/admin/purchasing'

function orderPath(supplierOrderId: string, rest = ''): string {
  return `${purchasing}/supplier-orders/${encodeURIComponent(supplierOrderId)}${rest}`
}

/** toBuy calls GET /api/v1/admin/purchasing/to-buy: "A comprar", by supplier. */
export function toBuy(client: HttpClient): Promise<ToBuy> {
  return client.get<ToBuy>(`${purchasing}/to-buy`)
}

/**
 * previewSupplierOrder calls POST …/supplier-orders/preview: the text the
 * supplier would get for the picked items. Nothing is written. A pick that is
 * not in "A comprar" any more throws `item_not_to_buy`.
 */
export function previewSupplierOrder(client: HttpClient, body: SupplierOrderRequest): Promise<SupplierMessagePreview> {
  return client.post<SupplierMessagePreview>(`${purchasing}/supplier-orders/preview`, body)
}

/**
 * sendSupplierOrder calls POST …/supplier-orders ("Enviar pelo WhatsApp"). It
 * takes the store's next C-number, so `idempotencyKey` is made once per tap
 * (`newIdempotencyKey`) and sent again, unchanged, on a retry: the retry then
 * answers the same pedido de compra. The server sends nothing; the app opens
 * the `whatsappUrl` that comes back.
 */
export function sendSupplierOrder(client: HttpClient, body: SupplierOrderRequest, idempotencyKey: string): Promise<SentSupplierOrder> {
  return client.post<SentSupplierOrder>(`${purchasing}/supplier-orders`, body, { headers: idempotencyHeaders(idempotencyKey) })
}

/** supplierOrders calls GET …/supplier-orders: "Enviadas", newest first. */
export function supplierOrders(client: HttpClient, filter: SupplierOrderFilter = {}): Promise<SupplierOrderPage> {
  const query = queryString({ state: filter.state, cursor: filter.cursor, limit: filter.limit })
  return client.get<SupplierOrderPage>(`${purchasing}/supplier-orders${query}`)
}

/** supplierOrder calls GET …/supplier-orders/{supplierOrderId}; an unknown id throws `not_found`. */
export function supplierOrder(client: HttpClient, supplierOrderId: string): Promise<SupplierOrder> {
  return client.get<SupplierOrder>(orderPath(supplierOrderId))
}

/**
 * recordPromisedDate calls PUT …/promised-date ("Prazo do fornecedor"): one
 * date for every open line and, in `lines`, a different one for some. A date
 * before today throws `invalid`; someone saving first throws `version_conflict`.
 */
export function recordPromisedDate(client: HttpClient, supplierOrderId: string, body: PromisedDateRequest): Promise<SupplierOrder> {
  return client.put<SupplierOrder>(orderPath(supplierOrderId, '/promised-date'), body)
}

/** markInTransit calls POST …/in-transit ("Marcar como em trânsito"). */
export function markInTransit(client: HttpClient, supplierOrderId: string, body: SupplierOrderVersionRequest): Promise<SupplierOrder> {
  return client.post<SupplierOrder>(orderPath(supplierOrderId, '/in-transit'), body)
}

/** undoSupplierOrder calls POST …/undo ("Desfazer envio"); once something was received it throws `supplier_order_received`. */
export function undoSupplierOrder(client: HttpClient, supplierOrderId: string, body: SupplierOrderVersionRequest): Promise<SupplierOrder> {
  return client.post<SupplierOrder>(orderPath(supplierOrderId, '/undo'), body)
}

/** recordPaidPrices calls PUT …/paid-prices ("Informar preço pago"). Owner only; a cancelled line throws `line_closed`. */
export function recordPaidPrices(client: HttpClient, supplierOrderId: string, body: PaidPricesRequest): Promise<SupplierOrder> {
  return client.put<SupplierOrder>(orderPath(supplierOrderId, '/paid-prices'), body)
}

/** notices calls GET …/notices: "Avisar o fornecedor". */
export function notices(client: HttpClient): Promise<NoticeList> {
  return client.get<NoticeList>(`${purchasing}/notices`)
}

/** resolveNotice calls POST …/notices/{lineId}/resolve ("Marcar como resolvido"); a line that is no longer a notice throws `line_closed`. */
export async function resolveNotice(client: HttpClient, lineId: string): Promise<void> {
  await client.post<unknown>(`${purchasing}/notices/${encodeURIComponent(lineId)}/resolve`, undefined)
}

/** orderPurchases calls GET …/orders/{orderId}/lines: what purchasing knows of one order's bought items (needs `order:read`). */
export function orderPurchases(client: HttpClient, orderId: string): Promise<OrderPurchases> {
  return client.get<OrderPurchases>(`${purchasing}/orders/${encodeURIComponent(orderId)}/lines`)
}

/** purchasingToday calls GET …/today: Hoje's purchasing entries, for `purchase:write` only. */
export function purchasingToday(client: HttpClient): Promise<PurchasingToday> {
  return client.get<PurchasingToday>(`${purchasing}/today`)
}
