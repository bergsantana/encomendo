import type { Alert, HistoryEntry, Order, OrderPhase } from '@encomendo/api'

/** OrderItem is one item of an order as the order screen shows it. */
export type OrderItem = Order['items'][number]
/** ItemStatus is where an order item stands in its sourcing mode's lifecycle. */
export type ItemStatus = OrderItem['status']
/** SourcingMode is how an item is obtained: bought to order, from stock, or made in house. */
export type SourcingMode = OrderItem['sourcingMode']
/** AlertKind is "Pode atrasar" (`at_risk`) or "Atrasada" (`late`). */
export type AlertKind = Alert['kind']
/** Handover is "Entrega" or "Retirada na loja". */
export type Handover = Order['handover']
/** ItemCheck is the result of checking an arrived item. */
export type ItemCheck = NonNullable<OrderItem['check']>
/** HistoryKind is what kind of event a history entry records. */
export type HistoryKind = HistoryEntry['kind']

/**
 * itemStatusLabel is the word for an item's situation, from the design
 * document's "Itens" table. Bought and stock items read "Recebida" and
 * "Instalada"; in-house items read "Pronto" and "Instalado".
 */
export function itemStatusLabel(status: ItemStatus, mode: SourcingMode): string {
  const inHouse = mode === 'in_house'
  const words: Record<ItemStatus, string> = {
    to_buy: 'A comprar',
    sent_to_supplier: 'Enviada ao fornecedor',
    in_transit: 'Em trânsito',
    received: 'Recebida',
    planned: 'Planejado',
    in_production: 'Em produção',
    ready: 'Pronto',
    installed: inHouse ? 'Instalado' : 'Instalada',
  }
  return words[status]
}

/**
 * itemTone is the order phase whose badge color an item status shares, from
 * the design document's status table ("A comprar / Planejado" grey, "Em
 * produção" and the wait for the supplier orange, "Pronto / Recebida" green,
 * installed like handed over). Items have no phase of their own.
 */
export function itemTone(status: ItemStatus): OrderPhase {
  const phases: Record<ItemStatus, OrderPhase> = {
    to_buy: 'draft',
    planned: 'draft',
    sent_to_supplier: 'sourcing',
    in_transit: 'sourcing',
    in_production: 'sourcing',
    received: 'ready',
    ready: 'ready',
    installed: 'handed_over',
  }
  return phases[status]
}

/** sourcingLabel is the "Origem" of an item. */
export function sourcingLabel(mode: SourcingMode): string {
  const words: Record<SourcingMode, string> = { to_order: 'Sob encomenda', stock: 'Estoque', in_house: 'Produção própria' }
  return words[mode]
}

/** handoverLabel is "Retirada na loja" or "Entrega". */
export function handoverLabel(handover: Handover): string {
  return handover === 'pickup' ? 'Retirada na loja' : 'Entrega'
}
