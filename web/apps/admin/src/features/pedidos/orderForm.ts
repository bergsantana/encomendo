import type { Item, Kit, LineRequest, Order, TermsInput } from '@encomendo/api'

import type { FieldCheck } from '../../composables/useForm'
import { centsToText, parseCents } from '../../format'
import type { Handover } from './labels'

/** CatalogOption is one customization option of a catalog item, as the picker shows it. */
export type CatalogOption = Item['options'][number]

/**
 * LineDraft is one line of the form: a kit or a loose item, its quantity as
 * typed, the swaps chosen on a kit and the choices made on the client's
 * options. It holds no price: the quote prices the line.
 */
export interface LineDraft {
  /** key identifies the line in the form only; the API never sees it. */
  key: string
  kind: 'kit' | 'item'
  /** catalogId is the kit's or the item's id. */
  catalogId: string
  name: string
  /** quantity is typed text, a whole number from 1 to 999 once checked. */
  quantity: string
  /** swaps maps a kit's item id to the alternative chosen in its place; a missing key means not swapped. */
  swaps: Record<string, string>
  /** choices maps `{itemId}:{optionId}` to the chosen choice id. */
  choices: Record<string, string>
}

/** TermsDraft is "Entrega" and "Valores" as typed. */
export interface TermsDraft {
  handover: Handover
  /** addressId is '' until one is chosen; it only counts for a delivery. */
  addressId: string
  freight: string
  discount: string
  depositPercent: string
  assemblyDays: string
  safetyMarginDays: string
}

/** OrderDraft is the whole form. */
export interface OrderDraft {
  lines: LineDraft[]
  terms: TermsDraft
  /** customerApproved is the box "O cliente aprovou a mudança". */
  customerApproved: boolean
}

/** newAddressChoice is the value of "Novo endereço" in the address choice, before that address is saved on the customer. */
export const newAddressChoice = 'new'

/** maxQuantity is the largest quantity a line takes. */
export const maxQuantity = 999

/** emptyTerms is the terms of a new order: pickup, and every value left to the store's default. */
export function emptyTerms(): TermsDraft {
  return { handover: 'pickup', addressId: '', freight: '', discount: '', depositPercent: '', assemblyDays: '', safetyMarginDays: '' }
}

/** emptyOrderDraft is the form of "Novo pedido". */
export function emptyOrderDraft(): OrderDraft {
  return { lines: [], terms: emptyTerms(), customerApproved: false }
}

/** optionKey names the choice made for one option of one item in a line's `choices`. */
export function optionKey(itemId: string, optionId: string): string {
  return `${itemId}:${optionId}`
}

/** wholeQuantity reads a quantity typed as digits, or null when it is not 1 to 999. */
export function wholeQuantity(text: string): number | null {
  if (!/^\d{1,4}$/.test(text.trim())) return null
  const value = Number(text.trim())
  return value >= 1 && value <= maxQuantity ? value : null
}

function wholeWithin(max: number): (text: string) => boolean {
  return (text) => text.trim() === '' || (/^\d{1,3}$/.test(text.trim()) && Number(text.trim()) <= max)
}

/** effectiveItems are the catalog items a line really holds: a kit's items with the swaps applied, or the one loose item. */
export function effectiveItems(line: LineDraft, kit: Kit | undefined): string[] {
  if (line.kind === 'item') return [line.catalogId]
  return (kit?.items ?? []).map((item) => line.swaps[item.itemId] ?? item.itemId)
}

/** toLineRequest builds one line of a quote or a save. A quantity that is not valid was stopped by the checks before. */
export function toLineRequest(line: LineDraft): LineRequest {
  const options = Object.entries(line.choices).map(([key, choiceId]) => {
    const [itemId = '', optionId = ''] = key.split(':')
    return { itemId, optionId, choiceId }
  })
  const base: LineRequest = { quantity: wholeQuantity(line.quantity) ?? 1, ...(options.length > 0 ? { options } : {}) }
  if (line.kind === 'item') return { ...base, itemId: line.catalogId }
  const swaps = Object.entries(line.swaps).map(([itemId, withItemId]) => ({ itemId, withItemId }))
  return { ...base, kitId: line.catalogId, ...(swaps.length > 0 ? { swaps } : {}) }
}

/** TermsAccess says which of "Valores" the form sends: only a role with `order:manage` sets discount, deposit percent and days. */
export interface TermsAccess {
  canManage: boolean
}

/**
 * toTerms builds the terms of a quote or a save. A field left empty is left
 * out, so the store's default (a new order) or the order's own value (a
 * change) stands. Freight counts only for a delivery. Staff without
 * `order:manage` never send discount, deposit percent or days: the API would
 * refuse them, and on a change leaving them out keeps the order's.
 */
export function toTerms(terms: TermsDraft, access: TermsAccess): TermsInput {
  const delivery = terms.handover === 'delivery'
  const request: TermsInput = { handover: terms.handover }
  if (delivery && terms.addressId !== '' && terms.addressId !== newAddressChoice) request.addressId = terms.addressId
  const freight = delivery ? parseCents(terms.freight) : null
  if (freight !== null) request.freightCents = freight
  if (!access.canManage) return request
  const discount = parseCents(terms.discount)
  if (discount !== null) request.discountCents = discount
  if (terms.depositPercent.trim() !== '') request.depositPercent = Number(terms.depositPercent)
  if (terms.assemblyDays.trim() !== '') request.assemblyDays = Number(terms.assemblyDays)
  if (terms.safetyMarginDays.trim() !== '') request.safetyMarginDays = Number(terms.safetyMarginDays)
  return request
}

/**
 * termsChecks are the checks on the typed values, run before a quote or a
 * save; the API checks the same limits again. Empty means "use the default".
 */
export function termsChecks(terms: TermsDraft, access: TermsAccess): FieldCheck[] {
  const money = (text: string) => text.trim() === '' || parseCents(text) !== null
  const checks: FieldCheck[] = [['freight', terms.freight, 'Confira o valor.', money]]
  if (!access.canManage) return checks
  return [
    ...checks,
    ['discount', terms.discount, 'Confira o valor.', money],
    ['depositPercent', terms.depositPercent, 'Use um sinal de 0 a 100%.', wholeWithin(100)],
    ['assemblyDays', terms.assemblyDays, 'Use de 0 a 60 dias.', wholeWithin(60)],
    ['safetyMarginDays', terms.safetyMarginDays, 'Use de 0 a 60 dias.', wholeWithin(60)],
  ]
}

/** termsFromOrder fills "Entrega" and "Valores" with what an order has, for "Alterar pedido". */
export function termsFromOrder(order: Order): TermsDraft {
  return {
    handover: order.handover,
    addressId: order.address?.id ?? '',
    freight: order.freight.cents === 0 ? '' : centsToText(order.freight.cents),
    discount: order.discount.cents === 0 ? '' : centsToText(order.discount.cents),
    depositPercent: String(order.depositPercent),
    assemblyDays: String(order.assemblyDays),
    safetyMarginDays: String(order.safetyMarginDays),
  }
}
