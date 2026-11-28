import { ApiError, type NoticeReason, PhotoFileError, PhotoUploadError, type PurchaseChoice, type PurchaseLine, type PurchaseLineStatus, type SupplierOrderState } from '@encomendo/api'
import type { Phase } from '@encomendo/ui'

import { errorMessage } from '../../errorMessage'

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

/** photoNotSent is what a photo that did not reach the server says. */
const photoNotSent = 'A foto não foi enviada. Tente de novo.'

/**
 * receiptPhotoMessage is what a failed receiving photo says, in the words of
 * Catálogo, or null when the person cancelled it. Anything that is not a
 * photo or API failure is a bug and is thrown on.
 */
export function receiptPhotoMessage(failure: unknown): string | null {
  if (failure instanceof PhotoFileError) return 'Use uma foto JPEG, PNG ou WebP de até 10 MB.'
  if (failure instanceof PhotoUploadError) return photoNotSent
  if (failure instanceof DOMException && failure.name === 'AbortError') return null
  if (!(failure instanceof ApiError)) throw failure
  return errorMessage(failure, { photo_not_uploaded: photoNotSent, photo_invalid: 'Use uma foto JPEG, PNG ou WebP de até 10 MB.' })
}

/** itemPhotoLimitMessage is the refusal of a third photo on one item. */
export const itemPhotoLimitMessage = 'Cada item pode ter até 2 fotos.'
/** invoicePhotoLimitMessage is the refusal of a fifth photo of the nota fiscal. */
export const invoicePhotoLimitMessage = 'Cada recebimento pode ter até 4 fotos da nota.'
/** totalPhotoLimitMessage is the refusal of a photo past the 20 a receipt holds; the design document does not word it. */
export const totalPhotoLimitMessage = 'Cada recebimento pode ter até 20 fotos no total.'

/** missingNoteMessage asks for the defect's note before "Salvar recebimento" calls the API. */
export function missingNoteMessage(itemName: string): string {
  return `Conte o que houve com ${itemName}.`
}

/** backToBuyMessage tells where a defective item went: back to "A comprar", its replacement in Compras. */
export function backToBuyMessage(itemName: string): string {
  return `${itemName} voltou para A comprar. A reposição está em Compras.`
}

/** completedOrderMessage tells that every item of an order has arrived and been checked. */
export function completedOrderMessage(orderNumber: string): string {
  return `Pedido ${orderNumber}: todos os itens chegaram e foram conferidos.`
}

/**
 * receiveClosedMessage is the refusal of a receipt that names a line already
 * received or cancelled (`line_closed`), or an order handed over or cancelled
 * (`order_closed`). The design document words neither for receiving, so it is
 * written like Compras' refusals; the list is read again with it.
 */
export const receiveClosedMessage = 'Algum item já foi recebido ou o pedido dele foi entregue ou cancelado. A lista foi atualizada; confira e salve de novo.'
