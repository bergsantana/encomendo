import type { HttpClient } from './http'
import { idempotencyHeaders } from './idempotency'
import { type PageRequest, queryString } from './query'
import type { components } from './types/schema'

type Schemas = components['schemas']

/** Order is one order with its lines, items, dates, alerts, payments and the moves it allows. Owner-only `cost` and `margin` are absent for other roles. */
export type Order = Schemas['Order']
/** OrderSummary is one row of the order list, Hoje and Financeiro. */
export type OrderSummary = Schemas['OrderSummary']
/** OrderPage is one page of orders, by delivery date. */
export type OrderPage = Schemas['OrderPage']
/** OrderPhase is the fixed phase a status belongs to; rules and badge colors hang on it. */
export type OrderPhase = Schemas['Phase']
/** Alert is a "Pode atrasar" or "Atrasada" warning about one order item. */
export type Alert = Schemas['Alert']
/** QuoteRequest is the body of POST /orders/quote. */
export type QuoteRequest = Schemas['QuoteRequest']
/** Quote is the price, deposit and dates the server computed for an order not yet saved. */
export type Quote = Schemas['Quote']
/** LineRequest is one line of an order: a kit with its swaps, or a loose item with a quantity, and the client's options. */
export type LineRequest = Schemas['LineRequest']
/** TermsInput is how the order is handed over and paid. */
export type TermsInput = Schemas['TermsInput']
/** CreateOrderRequest is the body of POST /orders. */
export type CreateOrderRequest = Schemas['CreateOrderRequest']
/** CreatedOrder is the new order, with its tracking link (this once) and, when sent to the customer, the wa.me link. */
export type CreatedOrder = Schemas['CreatedOrder']
/** ChangeOrderRequest is the body of PUT /orders/{orderId}. */
export type ChangeOrderRequest = Schemas['ChangeOrderRequest']
/** ChangedOrder is the order as saved, what changed, and for a paid order the wa.me link to tell the customer. */
export type ChangedOrder = Schemas['ChangedOrder']
/** AdvanceRequest moves an order to another status. */
export type AdvanceRequest = Schemas['AdvanceRequest']
/** MoveItemRequest moves one order item along its lifecycle. */
export type MoveItemRequest = Schemas['MoveItemRequest']
/** PaymentRequest is the body of recording a payment by hand. */
export type PaymentRequest = Schemas['PaymentRequest']
/** VoidPaymentRequest is the body of voiding a payment. */
export type VoidPaymentRequest = Schemas['VoidPaymentRequest']
/** DeliveryDateRequest changes the date promised to the client. */
export type DeliveryDateRequest = Schemas['DeliveryDateRequest']
/** CancelRequest is the body of cancelling an order. */
export type CancelRequest = Schemas['CancelRequest']
/** MessagedOrder is an order and the wa.me link of the message to send the client. */
export type MessagedOrder = Schemas['MessagedOrder']
/** TrackingLink is a new tracking link and the wa.me link carrying it. */
export type TrackingLink = Schemas['TrackingLink']
/** HistoryEntry is one event of an order. */
export type HistoryEntry = Schemas['HistoryEntry']
/** HistoryPage is one page of an order's history, newest first. */
export type HistoryPage = Schemas['HistoryPage']
/** Today is the Hoje screen: counts, what needs the team and the next deliveries. */
export type Today = Schemas['Today']
/** FinanceSummary is the Financeiro screen for a period. */
export type FinanceSummary = Schemas['FinanceSummary']
/** VersionRequest is the body of the calls that only need the order's `version`. */
export type OrderVersionRequest = Schemas['VersionRequest']

/** OrderListFilter narrows the order list. Left out, it lists active orders. */
export interface OrderListFilter extends PageRequest {
  /** status is a status key of the order's workflow. */
  status?: string
  phase?: OrderPhase
  scope?: 'active' | 'handed_over' | 'cancelled' | 'all'
  customerId?: string
  /** q is an order number (`231` or `P-0231`) or the start of the customer's name. */
  q?: string
  /** alert keeps only orders with "Pode atrasar" or "Atrasada" today. */
  alert?: boolean
}

/** Period is a range of days in the store's time zone, both ends included. */
export interface Period {
  from: string
  to: string
}

const admin = '/api/v1/admin'

function orderPath(orderId: string, rest = ''): string {
  return `${admin}/orders/${encodeURIComponent(orderId)}${rest}`
}

/**
 * quoteOrder calls POST /api/v1/admin/orders/quote: prices, deposit and dates
 * for an order not yet saved, or for a change when `orderId` is set. Nothing
 * is written; the screen shows what comes back and adds nothing up itself.
 */
export function quoteOrder(client: HttpClient, body: QuoteRequest, options: { signal?: AbortSignal } = {}): Promise<Quote> {
  return client.post<Quote>(`${admin}/orders/quote`, body, options)
}

/**
 * createOrder calls POST /api/v1/admin/orders. `idempotencyKey` is made once
 * per user action (`newIdempotencyKey`) and sent again, unchanged, on every
 * retry of it: the retry then answers the first result, without the tracking
 * link. A new action gets a new key; the same key with another body throws
 * `idempotency_key_reused`.
 */
export function createOrder(client: HttpClient, body: CreateOrderRequest, idempotencyKey: string): Promise<CreatedOrder> {
  return client.post<CreatedOrder>(`${admin}/orders`, body, { headers: idempotencyHeaders(idempotencyKey) })
}

/** listOrders calls GET /api/v1/admin/orders with the filter set. */
export function listOrders(client: HttpClient, filter: OrderListFilter = {}): Promise<OrderPage> {
  const query = queryString({
    cursor: filter.cursor,
    limit: filter.limit,
    status: filter.status,
    phase: filter.phase,
    scope: filter.scope,
    customerId: filter.customerId,
    q: filter.q,
    // Only a set filter is sent: "alert=false" would say nothing the default does not.
    alert: filter.alert === true ? true : undefined,
  })
  return client.get<OrderPage>(`${admin}/orders${query}`)
}

/** getOrder calls GET /api/v1/admin/orders/{orderId}; an unknown id throws `not_found`. */
export function getOrder(client: HttpClient, orderId: string): Promise<Order> {
  return client.get<Order>(orderPath(orderId))
}

/**
 * changeOrder calls PUT /api/v1/admin/orders/{orderId} with every line and
 * term and the `version` read. A paid order needs `customerApproved: true`
 * (`approval_required` otherwise); past assembling it throws `order_not_editable`.
 */
export function changeOrder(client: HttpClient, orderId: string, body: ChangeOrderRequest): Promise<ChangedOrder> {
  return client.put<ChangedOrder>(orderPath(orderId), body)
}

/**
 * advanceOrder calls POST …/advance. A guard that does not hold throws
 * `guard_failed`; the error's `guard` says which one did not hold.
 */
export function advanceOrder(client: HttpClient, orderId: string, body: AdvanceRequest): Promise<Order> {
  return client.post<Order>(orderPath(orderId, '/advance'), body)
}

/** releaseOrder calls POST …/release (owner): the order goes ahead without the deposit. */
export function releaseOrder(client: HttpClient, orderId: string, body: OrderVersionRequest): Promise<Order> {
  return client.post<Order>(orderPath(orderId, '/release'), body)
}

/** moveOrderItem calls POST …/items/{itemId}/move; `itemId` is the order's item, not the catalog's. */
export function moveOrderItem(client: HttpClient, orderId: string, itemId: string, body: MoveItemRequest): Promise<Order> {
  return client.post<Order>(orderPath(orderId, `/items/${encodeURIComponent(itemId)}/move`), body)
}

/** useBackupSupplier calls POST …/items/{itemId}/use-backup-supplier ("Usar o fornecedor reserva"). */
export function useBackupSupplier(client: HttpClient, orderId: string, itemId: string, body: OrderVersionRequest): Promise<Order> {
  return client.post<Order>(orderPath(orderId, `/items/${encodeURIComponent(itemId)}/use-backup-supplier`), body)
}

/**
 * recordPayment calls POST …/payments. Like createOrder it takes the key of
 * the user action and sends it again on a retry, so a payment is never
 * recorded twice.
 */
export function recordPayment(client: HttpClient, orderId: string, body: PaymentRequest, idempotencyKey: string): Promise<Order> {
  return client.post<Order>(orderPath(orderId, '/payments'), body, { headers: idempotencyHeaders(idempotencyKey) })
}

/** voidPayment calls POST …/payments/{paymentId}/void (owner). */
export function voidPayment(client: HttpClient, orderId: string, paymentId: string, body: VoidPaymentRequest): Promise<Order> {
  return client.post<Order>(orderPath(orderId, `/payments/${encodeURIComponent(paymentId)}/void`), body)
}

/** changeDeliveryDate calls PUT …/delivery-date; the answer carries the wa.me link to tell the client. */
export function changeDeliveryDate(client: HttpClient, orderId: string, body: DeliveryDateRequest): Promise<MessagedOrder> {
  return client.put<MessagedOrder>(orderPath(orderId, '/delivery-date'), body)
}

/** sendTrackingLink calls POST …/tracking-link: a new link, and the previous one stops working. */
export function sendTrackingLink(client: HttpClient, orderId: string): Promise<TrackingLink> {
  return client.post<TrackingLink>(orderPath(orderId, '/tracking-link'), undefined)
}

/** cancelOrder calls POST …/cancel; payments stay as recorded. */
export function cancelOrder(client: HttpClient, orderId: string, body: CancelRequest): Promise<Order> {
  return client.post<Order>(orderPath(orderId, '/cancel'), body)
}

/** orderHistory calls GET …/history, newest first. */
export function orderHistory(client: HttpClient, orderId: string, page: PageRequest = {}): Promise<HistoryPage> {
  return client.get<HistoryPage>(orderPath(orderId, `/history${queryString({ cursor: page.cursor, limit: page.limit })}`))
}

/** getToday calls GET /api/v1/admin/today (Hoje). */
export function getToday(client: HttpClient): Promise<Today> {
  return client.get<Today>(`${admin}/today`)
}

/** getFinanceSummary calls GET /api/v1/admin/finance/summary for a period (owner). */
export function getFinanceSummary(client: HttpClient, period: Period): Promise<FinanceSummary> {
  return client.get<FinanceSummary>(`${admin}/finance/summary${queryString({ from: period.from, to: period.to })}`)
}
