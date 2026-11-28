import {
  advanceOrder,
  ApiError,
  cancelOrder,
  changeDeliveryDate,
  getOrder,
  getSettings,
  type HistoryEntry,
  moveOrderItem,
  newIdempotencyKey,
  type Order,
  orderHistory,
  recordPayment,
  releaseOrder,
  sendTrackingLink,
  useBackupSupplier,
  voidPayment,
} from '@encomendo/api'
import { defineStore } from 'pinia'
import { computed, ref, shallowRef } from 'vue'

import { api } from '../../api'
import { errorMessage } from '../../errorMessage'
import { formatCivilDate, formatMoney } from '../../format'
import { type Handover, type ItemCheck, type ItemStatus, itemStatusLabel, type PaymentKind, type PaymentMethod } from './labels'
import { openWhatsapp } from './openLink'

const conflictMessage = 'Outra pessoa alterou este pedido. Recarregue para ver a versão atual.'
const historySize = 50

/** PaymentInput is what "Registrar pagamento" sends, with the amount already in cents. */
export interface PaymentInput {
  kind: PaymentKind
  amountCents: number
  method: PaymentMethod
}

/**
 * useOrderStore backs the order screen: one order with its history and
 * every action on it (move an item, advance, record or void a payment,
 * release, change the date, send the tracking link, cancel). Every write
 * sends the `version` read, and the answer replaces the order, so the screen
 * always shows what the API says; nothing is worked out here. The wa.me
 * links are the API's and are only opened. A payment keeps its
 * `Idempotency-Key` across retries until its input changes or it succeeds.
 */
export const useOrderStore = defineStore('order', () => {
  const order = shallowRef<Order | null>(null)
  const loading = ref(false)
  const loadError = ref<string | null>(null)
  /** timeZone is the store's, for the dates of instants such as "Criado em"; undefined until read. */
  const timeZone = ref<string | undefined>(undefined)
  const busy = ref(false)
  /** notice is what the last action did, such as "Pedido cancelado." */
  const notice = ref<string | null>(null)
  /** error is the refusal of an action done from the page; `dialogError` is that of an action done in a dialog. */
  const error = ref<string | null>(null)
  const dialogError = ref<string | null>(null)
  /** guardMessage is why "Mover para …" was refused, under the buttons. */
  const guardMessage = ref<string | null>(null)
  const conflict = ref(false)

  const history = shallowRef<HistoryEntry[]>([])
  const historyCursor = ref<string | undefined>(undefined)
  const historyLoading = ref(false)
  const historyError = ref<string | null>(null)

  let paymentKey: string | null = null
  let paymentBody = ''
  let historyRead = 0
  let openRead = 0

  function currentOrder(): Order {
    if (order.value === null) throw new Error('no order is open')
    return order.value
  }

  // ---- Reading ----

  async function readHistory(cursor: string | undefined): Promise<void> {
    const current = order.value
    if (current === null) return
    const read = ++historyRead
    historyLoading.value = true
    historyError.value = null
    try {
      const page = await orderHistory(api, current.id, { cursor, limit: historySize })
      if (read !== historyRead) return
      history.value = cursor === undefined ? page.items : [...history.value, ...page.items]
      historyCursor.value = page.nextCursor
    } catch (failure) {
      if (!(failure instanceof ApiError)) throw failure
      if (read === historyRead) historyError.value = errorMessage(failure)
    } finally {
      if (read === historyRead) historyLoading.value = false
    }
  }

  function reset(): void {
    order.value = null
    loadError.value = null
    notice.value = null
    error.value = null
    dialogError.value = null
    guardMessage.value = null
    conflict.value = false
    history.value = []
    historyCursor.value = undefined
    historyError.value = null
    paymentKey = null
    paymentBody = ''
    historyRead += 1
    openRead += 1
  }

  /** open reads the order, the store's time zone and the history. */
  async function open(orderId: string): Promise<void> {
    reset()
    const mine = openRead
    loading.value = true
    try {
      const [read, settings] = await Promise.all([getOrder(api, orderId), getSettings(api).catch(() => null)])
      // Another order was opened meanwhile: its answer is the one to show.
      if (mine !== openRead) return
      order.value = read
      timeZone.value = settings?.timeZone
    } catch (failure) {
      if (!(failure instanceof ApiError)) throw failure
      loadError.value = errorMessage(failure, { not_found: 'Pedido não encontrado.' })
      return
    } finally {
      loading.value = false
    }
    await readHistory(undefined)
  }

  /** reload reads the open order again ("Recarregar"), dropping every message. */
  async function reload(): Promise<void> {
    const current = order.value
    if (current !== null) await open(current.id)
  }

  /** loadMoreHistory appends the next page of the history. */
  async function loadMoreHistory(): Promise<void> {
    if (historyCursor.value !== undefined) await readHistory(historyCursor.value)
  }

  // ---- Writing ----

  /**
   * act runs a write and, on success, takes the order it answered. A refusal
   * becomes the message above the page, or in the dialog when `inDialog`;
   * `own` is the screen's wording for the codes it expects. It resolves
   * whether the write worked.
   */
  async function act<T>(write: () => Promise<T>, take: (answer: T) => Order, options: { inDialog?: boolean; own?: Partial<Record<string, string>> } = {}): Promise<boolean> {
    busy.value = true
    error.value = null
    dialogError.value = null
    guardMessage.value = null
    conflict.value = false
    try {
      const answer = await write()
      order.value = take(answer)
      void readHistory(undefined)
      return true
    } catch (failure) {
      if (!(failure instanceof ApiError)) throw failure
      conflict.value = failure.code === 'version_conflict'
      const message = errorMessage(failure, { version_conflict: conflictMessage, ...options.own })
      // A conflict is shown on the page with "Recarregar", even from a dialog: every retry would send the same stale version.
      if (options.inDialog === true && !conflict.value) dialogError.value = message
      else error.value = message
      return false
    } finally {
      busy.value = false
    }
  }

  /** guardText is why the move was refused, in the design document's words. */
  function guardText(guard: string | undefined, current: Order): string | null {
    if (guard === 'deposit_unpaid') return 'O sinal ainda não foi pago. Registre o sinal ou libere o pedido sem ele.'
    if (guard === 'items_not_ready') return 'Nem todos os itens chegaram e foram conferidos.'
    if (guard === 'balance_unpaid') return `Falta receber o saldo de ${formatMoney(current.balanceDue.cents, current.balanceDue.currency)}.`
    return null
  }

  /**
   * advance moves the order to a status of its workflow. A guard that does
   * not hold shows its own message under the buttons; the handover status
   * also sends how the order was handed over.
   */
  async function advance(to: string, handover?: Handover): Promise<boolean> {
    const current = currentOrder()
    notice.value = null
    const status = current.moves.find((move) => move.key === to)
    busy.value = true
    error.value = null
    dialogError.value = null
    guardMessage.value = null
    conflict.value = false
    try {
      order.value = await advanceOrder(api, current.id, { to, version: current.version, ...(handover === undefined ? {} : { handover }) })
      void readHistory(undefined)
      return true
    } catch (failure) {
      if (!(failure instanceof ApiError)) throw failure
      conflict.value = failure.code === 'version_conflict'
      if (failure.code === 'guard_failed') guardMessage.value = guardText(failure.guard, current) ?? errorMessage(failure)
      else if (failure.code === 'invalid_transition') guardMessage.value = `Este pedido não pode ir para ${status?.label ?? 'este status'}.`
      else error.value = errorMessage(failure, { version_conflict: conflictMessage })
      return false
    } finally {
      busy.value = false
    }
  }

  /**
   * moveItem moves one item along its lifecycle. A defective item goes back,
   * and the screen says where: "{item} voltou para A comprar."
   */
  async function moveItem(itemId: string, to: ItemStatus, check?: ItemCheck): Promise<boolean> {
    const current = currentOrder()
    notice.value = null
    const moved = await act(
      () => moveOrderItem(api, current.id, itemId, { to, version: current.version, ...(check === undefined ? {} : { check }) }),
      (answer) => answer,
    )
    if (!moved || check !== 'defective') return moved
    const back = order.value?.items.find((item) => item.id === itemId)
    if (back !== undefined) notice.value = `${back.name} voltou para ${itemStatusLabel(back.status, back.sourcingMode)}.`
    return moved
  }

  /** switchSupplier moves a bought item to its backup supplier ("Usar o fornecedor reserva"). */
  async function switchSupplier(itemId: string): Promise<boolean> {
    const current = currentOrder()
    notice.value = null
    const switched = await act(
      () => useBackupSupplier(api, current.id, itemId, { version: current.version }),
      (answer) => answer,
      { own: { no_backup_supplier: 'Este item não tem fornecedor reserva ou já foi comprado.' } },
    )
    const item = order.value?.items.find((candidate) => candidate.id === itemId)
    if (switched && item?.supplier !== undefined) notice.value = `${item.name} agora vem de ${item.supplier.name}. Previsto: ${formatCivilDate(item.expectedDate)}.`
    return switched
  }

  /**
   * pay records a payment. The key of the user action is made when the input
   * is new and kept while it stays the same, so a retry after a lost answer
   * never records it twice.
   */
  async function pay(input: PaymentInput): Promise<boolean> {
    const current = currentOrder()
    notice.value = null
    const body = { ...input, version: current.version }
    const text = JSON.stringify(body)
    if (paymentKey === null || text !== paymentBody) {
      paymentKey = newIdempotencyKey()
      paymentBody = text
    }
    const key = paymentKey
    const paid = await act(
      () => recordPayment(api, current.id, body, key),
      (answer) => answer,
      {
        inDialog: true,
        own: {
          payment_exceeds_due: `O valor passa do que falta pagar (${formatMoney(current.balanceDue.cents, current.balanceDue.currency)}).`,
          invalid: 'Informe um valor maior que zero.',
          idempotency_key_reused: 'Esta chave já foi usada em outro pedido.',
        },
      },
    )
    if (paid) {
      paymentKey = null
      notice.value = 'Pagamento registrado.'
    }
    return paid
  }

  /** voidPayment voids a wrong payment with its reason (owner). */
  async function annul(paymentId: string, reason: string): Promise<boolean> {
    const current = currentOrder()
    notice.value = null
    const done = await act(() => voidPayment(api, current.id, paymentId, { reason, version: current.version }), (answer) => answer, {
      inDialog: true,
      own: { invalid: 'Informe o motivo.' },
    })
    if (done) notice.value = 'Pagamento anulado.'
    return done
  }

  /** release lets the order go ahead without the deposit (owner). */
  async function release(): Promise<boolean> {
    const current = currentOrder()
    notice.value = null
    const done = await act(() => releaseOrder(api, current.id, { version: current.version }), (answer) => answer, { inDialog: true })
    if (done) notice.value = 'Pedido liberado.'
    return done
  }

  /** changeDate moves the promised date and opens the wa.me link that tells the client. */
  async function changeDate(date: string, reason: string): Promise<boolean> {
    const current = currentOrder()
    notice.value = null
    const text = reason.trim()
    const done = await act(
      () => changeDeliveryDate(api, current.id, { date, version: current.version, ...(text === '' ? {} : { reason: text }) }),
      (answer) => {
        openWhatsapp(answer.whatsappUrl)
        return answer.order
      },
      { inDialog: true, own: { invalid: 'Escolha uma data a partir de hoje.' } },
    )
    if (done) notice.value = 'Data alterada. Envie a mensagem para avisar o cliente.'
    return done
  }

  /** sendTracking issues a new tracking link and opens WhatsApp with it; the previous link stops working. */
  async function sendTracking(): Promise<boolean> {
    const current = currentOrder()
    notice.value = null
    busy.value = true
    error.value = null
    try {
      const link = await sendTrackingLink(api, current.id)
      openWhatsapp(link.whatsappUrl)
      void readHistory(undefined)
      return true
    } catch (failure) {
      if (!(failure instanceof ApiError)) throw failure
      error.value = errorMessage(failure)
      return false
    } finally {
      busy.value = false
    }
  }

  /** cancel cancels the order with its reason; the payments stay as recorded. */
  async function cancel(reason: string): Promise<boolean> {
    const current = currentOrder()
    notice.value = null
    const done = await act(() => cancelOrder(api, current.id, { reason: reason.trim(), version: current.version }), (answer) => answer, {
      inDialog: true,
      own: { invalid: 'Informe o motivo.' },
    })
    if (done) notice.value = 'Pedido cancelado.'
    return done
  }

  /** clearDialogError forgets the refusal shown in a dialog, when it is opened or closed. */
  function clearDialogError(): void {
    dialogError.value = null
  }

  return {
    order,
    loading,
    loadError,
    timeZone,
    busy,
    notice,
    error,
    dialogError,
    guardMessage,
    conflict,
    history,
    hasMoreHistory: computed(() => historyCursor.value !== undefined),
    historyLoading,
    historyError,
    open,
    reload,
    loadMoreHistory,
    advance,
    moveItem,
    switchSupplier,
    pay,
    annul,
    release,
    changeDate,
    sendTracking,
    cancel,
    clearDialogError,
  }
})
