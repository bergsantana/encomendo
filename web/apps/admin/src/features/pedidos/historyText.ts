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
 * the entry has none.
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
      return `${who} marcou ${entry.itemName ?? 'um item'}${itemMove(entry, items)}`
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
  }
}

function itemMove(entry: HistoryEntry, items: Order['items']): string {
  if (entry.itemTo === undefined) return ''
  const mode = items.find((item) => item.id === entry.itemId)?.sourcingMode ?? 'to_order'
  const check = entry.check === 'defective' ? ' (com defeito)' : ''
  return ` como ${itemStatusLabel(entry.itemTo, mode).toLowerCase()}${check}`
}
