import type { HistoryEntry, Order } from '@encomendo/api'

import { formatCivilDate, formatMoney } from '../../format'
import { paymentMethodLabel } from '../../paymentLabels'
import { itemStatusLabel } from './labels'

/**
 * historyText writes one history entry as "{quem} {o quê}", such as "Ana
 * moveu para Em montagem" or "Ana registrou o sinal de R$ 300,00 (Pix)". The
 * status words in it are the ones the entry carries from the order's own
 * workflow. An entry with no author (something the system did) starts with
 * "O sistema". Item moves find the item's sourcing mode in the order, since
 * the entry has none. The moves of a bought item that Compras and Receber
 * itens make read as the design document's Histórico does ("Ana enviou {item}
 * ao fornecedor (C-0012)", "Ana recebeu {item} com defeito: {nota}").
 */
export function historyText(entry: HistoryEntry, items: Order['items']): string {
  const who = entry.by?.name ?? 'O sistema'
  const money = entry.amount === undefined ? '' : formatMoney(entry.amount.cents, entry.amount.currency)
  switch (entry.kind) {
    case 'created':
      return `${who} criou o pedido`
    case 'status':
      return `${who} moveu para ${entry.to?.label ?? ''}`.trim()
    case 'item':
      return purchasingMove(who, entry, items) ?? `${who} marcou ${entry.itemName ?? 'um item'}${itemMove(entry, items)}`
    case 'backup_supplier':
      return `${who} usou o fornecedor reserva em ${entry.itemName ?? 'um item'}`
    case 'payment':
      return `${who} registrou ${entry.paymentKind === 'deposit' ? 'o sinal' : 'o saldo'} de ${money}${entry.paymentMethod === undefined ? '' : ` (${paymentMethodLabel(entry.paymentMethod)})`}`
    case 'payment_voided':
      return `${who} anulou o pagamento de ${money}${entry.reason ? `: ${entry.reason}` : ''}`
    case 'released':
      return `${who} liberou o pedido sem o sinal`
    case 'date':
      return `${who} alterou a entrega de ${entry.oldDate === undefined ? '' : formatCivilDate(entry.oldDate)} para ${entry.newDate === undefined ? '' : formatCivilDate(entry.newDate)}${entry.reason ? `. Motivo: ${entry.reason}` : ''}`
    case 'changed':
      return `${who} alterou o pedido${entry.customerApproved === true ? ' (cliente aprovou)' : ''}`
    case 'tracking_link':
      return `${who} enviou o link de acompanhamento`
    case 'cancelled':
      return `${who} cancelou${entry.reason ? `: ${entry.reason}` : ''}`
    case 'promised_date':
      return `${who} informou o prazo do fornecedor de ${entry.itemName ?? 'um item'}: ${entry.newDate === undefined ? '' : formatCivilDate(entry.newDate)}`
  }
}

function itemMove(entry: HistoryEntry, items: Order['items']): string {
  if (entry.itemTo === undefined) return ''
  const mode = items.find((item) => item.id === entry.itemId)?.sourcingMode ?? 'to_order'
  const check = entry.check === 'defective' ? ' (com defeito)' : ''
  return ` como ${itemStatusLabel(entry.itemTo, mode).toLowerCase()}${check}`
}

/** purchased are the states a bought item reaches through Compras and Receber itens. */
const purchased: readonly NonNullable<HistoryEntry['itemTo']>[] = ['sent_to_supplier', 'in_transit', 'received', 'to_buy']

/**
 * purchasingMove words a move of a bought item that purchasing made: the
 * send, the transit mark, a receipt with its check, or an undone send (a move
 * back to "A comprar" with no check). `ref` is the pedido de compra, absent
 * for an item received at the counter. It is null for any other move.
 */
function purchasingMove(who: string, entry: HistoryEntry, items: Order['items']): string | null {
  if (entry.itemTo === undefined || !purchased.includes(entry.itemTo)) return null
  if ((items.find((item) => item.id === entry.itemId)?.sourcingMode ?? 'to_order') === 'in_house') return null
  const item = entry.itemName ?? 'um item'
  const number = entry.ref === undefined ? '' : ` (${entry.ref})`
  if (entry.check === 'defective') return `${who} recebeu ${item} com defeito${entry.reason ? `: ${entry.reason}` : ''}`
  switch (entry.itemTo) {
    case 'sent_to_supplier':
      return `${who} enviou ${item} ao fornecedor${number}`
    case 'in_transit':
      return `${who} marcou ${item} como em trânsito`
    case 'received':
      return `${who} recebeu ${item}${entry.check === 'ok' ? ' (tudo certo)' : ''}`
    default:
      return `${who} desfez o envio de ${item}${number}`
  }
}
