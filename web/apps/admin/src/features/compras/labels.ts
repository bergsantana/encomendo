import type { NoticeReason, PurchaseChoice, PurchaseLine, PurchaseLineStatus, SupplierOrderState } from '@encomendo/api'
import type { Phase } from '@encomendo/ui'

/** conflictMessage is what an edit to a pedido de compra that someone else changed first says; "Recarregar" goes with it. */
export const conflictMessage = 'Outra pessoa alterou este pedido de compra. Recarregue para ver a versão atual.'

/** receivedMessage is the refusal of "Desfazer envio" once something was received (`supplier_order_received`). */
export const receivedMessage = 'Este pedido de compra já tem itens recebidos e não pode ser desfeito.'

/** notToBuyMessage is the refusal of a send whose pick changed meanwhile (`item_not_to_buy`); the list is read again with it. */
export const notToBuyMessage = 'Algum item marcado já não está em A comprar. A lista foi atualizada; confira e envie de novo.'

/** lineClosedMessage is the refusal of a write on a line that was received, found defective or cancelled meanwhile (`line_closed`). */
export const lineClosedMessage = 'Esta linha já foi recebida ou cancelada. Recarregue para ver a situação atual.'

/** orderClosedMessage is the refusal of a send for an order handed over or cancelled meanwhile (`order_closed`). */
export const orderClosedMessage = 'Um pedido desta compra foi entregue ou cancelado. A lista foi atualizada; confira e envie de novo.'

/** lineStatusLabel is a line's "Situação" in the pedido de compra. */
export function lineStatusLabel(status: PurchaseLineStatus): string {
  const labels: Record<PurchaseLineStatus, string> = {
    sent: 'Enviada ao fornecedor',
    in_transit: 'Em trânsito',
    received: 'Recebida',
    defective: 'Com defeito (reposição em Compras)',
    cancelled: 'Cancelada',
  }
  return labels[status]
}

/**
 * lineTone borrows a phase for the badge's color and icon: a line waits on
 * the supplier like an order in sourcing, a defective one waits for its
 * replacement the same way, a received one is "Pronto / Recebida" green, and
 * a cancelled one is neutral. Only the badge reads it.
 */
export function lineTone(status: PurchaseLineStatus): Phase {
  if (status === 'sent' || status === 'in_transit' || status === 'defective') return 'sourcing'
  if (status === 'received') return 'ready'
  return 'cancelled'
}

/** orderStateLabel is a pedido de compra's "Situação" in "Enviadas". */
export function orderStateLabel(state: SupplierOrderState): string {
  const labels: Record<SupplierOrderState, string> = {
    sent: 'Enviado',
    in_transit: 'Em trânsito',
    received: 'Recebido',
    undone: 'Desfeito',
  }
  return labels[state]
}

/** orderStateTone is the badge phase of a pedido de compra's state, as `lineTone` is for a line. */
export function orderStateTone(state: SupplierOrderState): Phase {
  if (state === 'sent' || state === 'in_transit') return 'sourcing'
  if (state === 'received') return 'ready'
  return 'cancelled'
}

/** choicesText writes a bought item's choices as "Cor: Preto, Tamanho: M", or '' when it has none. */
export function choicesText(choices: readonly PurchaseChoice[]): string {
  return choices.map((choice) => `${choice.name}: ${choice.label}`).join(', ')
}

/** itemsCount is "1 item" or "{n} itens"; it counts entries, never summed quantities. */
export function itemsCount(count: number): string {
  return count === 1 ? '1 item' : `${count} itens`
}

/**
 * noticeReasonText says why a sent line is no longer needed. The design
 * document words the first two; `item_changed` is worded in the same style.
 */
export function noticeReasonText(reason: NoticeReason, orderNumber: string, itemName: string): string {
  if (reason === 'order_cancelled') return `O pedido ${orderNumber} foi cancelado.`
  if (reason === 'item_removed') return `${itemName} saiu do pedido ${orderNumber}.`
  return `${itemName} mudou no pedido ${orderNumber}.`
}

/** noticesHeading is the line above the notices: "{n} itens já enviados não são mais necessários." */
export function noticesHeading(count: number): string {
  return count === 1 ? '1 item já enviado não é mais necessário.' : `${count} itens já enviados não são mais necessários.`
}

/** lineAlert is the alert a line shows, "Atrasada" over "Pode atrasar", or null when it has none. */
export function lineAlert(line: Pick<PurchaseLine, 'alerts'>): 'late' | 'at_risk' | null {
  if (line.alerts.includes('late')) return 'late'
  return line.alerts.length > 0 ? 'at_risk' : null
}
