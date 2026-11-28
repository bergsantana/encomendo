import {
  ApiError,
  createSupplier,
  getSettings,
  getSupplier,
  listSuppliers,
  markInTransit,
  newIdempotencyKey,
  type Notice,
  notices as readNotices,
  previewSupplierOrder,
  recordPaidPrices,
  recordPromisedDate,
  replaceSupplier,
  resolveNotice,
  sendSupplierOrder,
  type SentSupplierOrder,
  type Supplier,
  type SupplierItemUse,
  supplierItems,
  type SupplierOrder,
  supplierOrder as readSupplierOrder,
  type SupplierOrderSummary,
  supplierOrders,
  toBuy,
  type ToBuySupplier,
  undoSupplierOrder,
} from '@encomendo/api'
import { defineStore } from 'pinia'
import { computed, ref, shallowRef } from 'vue'

import { api } from '../../api'
import { copyText } from '../../clipboard'
import { useForm } from '../../composables/useForm'
import { errorMessage } from '../../errorMessage'
import { openWhatsapp } from '../../openLink'
import { whatsappDigits } from '../../phone'
import { useSessionStore } from '../../stores/session'
import { supplierChecks } from '../catalogo/supplierChecks'
import { conflictMessage, lineClosedMessage, notToBuyMessage, orderClosedMessage, receivedMessage } from './labels'

const pageSize = 30
const supplierPageSize = 50
const fixTheFields = 'Confira os campos destacados.'
const supplierConflictMessage = 'Outra pessoa alterou este fornecedor. Recarregue para ver a versão atual.'

/** ComprasTab is a tab of Compras; `?tab=` carries it. */
export type ComprasTab = 'a-comprar' | 'enviadas' | 'receber' | 'fornecedores'

/** SentOrder is what "Enviar pelo WhatsApp" did, kept on the screen until the next action. */
export interface SentOrder {
  supplierOrderId: string
  number: string
  supplierName: string
  whatsappUrl: string
  text: string
}

/** CopyStatus is what "Copiar mensagem" did and where to say it: on a supplier's card, or on the "criado" block when `supplierId` is null. */
export interface CopyStatus {
  supplierId: string | null
  text: string
}

/** CardError is the refusal of a send, for the card of the supplier it was for. */
export interface CardError {
  supplierId: string
  message: string
}

/** Preview is the message of "Ver mensagem" for one supplier's checked items. */
export interface Preview {
  supplierId: string
  text: string | null
  loading: boolean
  error: string | null
}

/** DelayWarning is a line the new promised date left "Pode atrasar", for "Veja o pedido para resolver". */
export interface DelayWarning {
  lineId: string
  itemName: string
  orderId: string
  orderNumber: string
}

/** OrderOptions say how a write on the open pedido de compra reports its refusals and its success. */
interface OrderWrite {
  /** inDialog shows a refusal in the dialog that made the call, except a conflict, which is always on the page. */
  inDialog?: boolean
  own?: Partial<Record<string, string>>
  /** done is the notice of a write that went through. */
  done: string
}

/** failureOf narrows a thrown value to the API's refusal; a programming error is thrown on. */
function failureOf(error: unknown): ApiError {
  if (!(error instanceof ApiError)) throw error
  return error
}

/**
 * useComprasStore backs Compras, the feature's one store: "A comprar" (picks,
 * the message preview and the send), "Enviadas", the notices of "Avisar o
 * fornecedor", the pedido de compra screen with its writes, and the
 * Fornecedores tab with its drawer. Everything shown is what the API sent:
 * the message, the numbers and the dates are never worked out here, and a
 * wa.me link is only opened. The send keeps its `Idempotency-Key` across
 * retries until its picks change or it succeeds.
 */
export const useComprasStore = defineStore('compras', () => {
  const session = useSessionStore()

  /** timeZone is the store's, for the day of "Enviado em"; undefined until read, then the viewer's stands in. */
  const timeZone = ref<string | undefined>(undefined)
  /** purchaseBeforeDeposit picks the hint of an empty "A comprar". */
  const purchaseBeforeDeposit = ref(false)
  /** sendsCosts says whether the signed-in role may write the paid price: the owner only (ADR 0031). */
  const sendsCosts = computed(() => session.role === 'owner')

  // ---- A comprar ----

  const suppliers = shallowRef<ToBuySupplier[]>([])
  const truncated = ref(false)
  const toBuyLoading = ref(false)
  /** toBuyLoaded is true once a read succeeded, so the screen tells "loading" from "empty". */
  const toBuyLoaded = ref(false)
  const toBuyError = ref<string | null>(null)
  /** unchecked holds the order items the person took the box off; every other item is picked. */
  const unchecked = shallowRef<ReadonlySet<string>>(new Set())
  const preview = ref<Preview | null>(null)
  const sending = ref(false)
  const sent = shallowRef<SentOrder | null>(null)
  /**
   * copyStatus is what "Copiar mensagem" did, for a status line on the card
   * it was done from (`supplierId`), or on the "criado" block when null.
   */
  const copyStatus = shallowRef<CopyStatus | null>(null)
  /** cardError is the refusal of a send or a preview, shown on the card of the supplier it was for. */
  const cardError = shallowRef<CardError | null>(null)
  let sendKey: string | null = null
  let sendBody = ''
  let previewRead = 0
  let toBuyRead = 0

  function pickedIds(supplierId: string): string[] {
    const supplier = suppliers.value.find((candidate) => candidate.supplierId === supplierId)
    return (supplier?.items ?? []).map((item) => item.orderItemId).filter((id) => !unchecked.value.has(id))
  }

  async function readToBuy(): Promise<void> {
    const read = ++toBuyRead
    toBuyLoading.value = true
    try {
      const answer = await toBuy(api)
      // A newer read started meanwhile: its answer is the one to show.
      if (read !== toBuyRead) return
      suppliers.value = answer.suppliers
      truncated.value = answer.truncated
      toBuyLoaded.value = true
      // A pick of an item that left the list is forgotten, so the set cannot grow forever.
      const here = new Set(answer.suppliers.flatMap((supplier) => supplier.items.map((item) => item.orderItemId)))
      unchecked.value = new Set([...unchecked.value].filter((id) => here.has(id)))
    } catch (failure) {
      if (read === toBuyRead) toBuyError.value = errorMessage(failureOf(failure))
    } finally {
      if (read === toBuyRead) toBuyLoading.value = false
    }
  }

  /** readSettings reads the store's time zone and "Liberar compras antes do sinal"; failing is harmless, the screen then uses defaults. */
  async function readSettings(): Promise<void> {
    try {
      const settings = await getSettings(api)
      timeZone.value = settings.timeZone
      purchaseBeforeDeposit.value = settings.purchaseBeforeDeposit
    } catch (failure) {
      failureOf(failure)
    }
  }

  /**
   * loadToBuy reads "A comprar" again, keeping the boxes the person took off.
   * What the last visit left on screen (the "criado" block, a message, an open
   * preview) is dropped, so nothing stale shows after coming back to Compras.
   */
  async function loadToBuy(): Promise<void> {
    toBuyError.value = null
    cardError.value = null
    copyStatus.value = null
    sent.value = null
    preview.value = null
    previewRead += 1
    await Promise.all([readToBuy(), readSettings()])
  }

  /** setPicked checks or unchecks one item; an open preview of its supplier is read again for the new picks. */
  function setPicked(supplierId: string, orderItemId: string, picked: boolean): void {
    const next = new Set(unchecked.value)
    if (picked) next.delete(orderItemId)
    else next.add(orderItemId)
    unchecked.value = next
    if (preview.value?.supplierId === supplierId) void readPreview(supplierId)
  }

  async function readPreview(supplierId: string): Promise<void> {
    const ids = pickedIds(supplierId)
    const read = ++previewRead
    // Nothing is checked: there is no message to show, so the preview closes.
    if (ids.length === 0) {
      preview.value = null
      return
    }
    preview.value = { supplierId, text: preview.value?.text ?? null, loading: true, error: null }
    try {
      const answer = await previewSupplierOrder(api, { supplierId, orderItemIds: ids })
      if (read === previewRead) preview.value = { supplierId, text: answer.text, loading: false, error: null }
    } catch (failure) {
      if (read === previewRead) preview.value = { supplierId, text: null, loading: false, error: refusalOfSend(failureOf(failure)) }
    }
  }

  /** togglePreview opens "Ver mensagem" for a supplier, with the text the API built for the checked items, or closes it. */
  async function togglePreview(supplierId: string): Promise<void> {
    copyStatus.value = null
    cardError.value = null
    if (preview.value?.supplierId === supplierId) {
      previewRead += 1
      preview.value = null
      return
    }
    preview.value = null
    await readPreview(supplierId)
  }

  function refusalOfSend(failure: ApiError): string {
    return errorMessage(failure, { item_not_to_buy: notToBuyMessage, order_closed: orderClosedMessage })
  }

  /**
   * send makes the pedido de compra of a supplier's checked items and opens
   * the wa.me link the API answers with. The link also stays on the screen
   * ("Abrir o WhatsApp"), because a browser may refuse to open it once the
   * tap's user activation is gone. Resolves whether it was sent.
   */
  async function send(supplierId: string): Promise<boolean> {
    const ids = pickedIds(supplierId)
    const supplier = suppliers.value.find((candidate) => candidate.supplierId === supplierId)
    if (ids.length === 0 || supplier === undefined || sending.value) return false
    const body = { supplierId, orderItemIds: ids }
    const text = JSON.stringify(body)
    if (sendKey === null || text !== sendBody) {
      sendKey = newIdempotencyKey()
      sendBody = text
    }
    sending.value = true
    sent.value = null
    copyStatus.value = null
    cardError.value = null
    toBuyError.value = null
    try {
      const answer = await sendSupplierOrder(api, body, sendKey)
      sent.value = sentOrder(answer, supplier.supplierName)
      sendKey = null
      sendBody = ''
      preview.value = null
      previewRead += 1
      openWhatsapp(answer.whatsappUrl)
      void readToBuy()
      return true
    } catch (failure) {
      const refused = failureOf(failure)
      cardError.value = { supplierId, message: refusalOfSend(refused) }
      // The picks are stale: the list is read again so the person sees what is still there.
      if (refused.code === 'item_not_to_buy' || refused.code === 'order_closed') void readToBuy()
      return false
    } finally {
      sending.value = false
    }
  }

  function sentOrder(answer: SentSupplierOrder, supplierName: string): SentOrder {
    return {
      supplierOrderId: answer.supplierOrder.id,
      number: answer.supplierOrder.number,
      supplierName,
      whatsappUrl: answer.whatsappUrl,
      text: answer.text,
    }
  }

  /** openSentLink opens the last send's wa.me link again ("Abrir o WhatsApp"). */
  function openSentLink(): void {
    if (sent.value !== null) openWhatsapp(sent.value.whatsappUrl)
  }

  /** copyMessage copies a message to the clipboard ("Copiar mensagem") and says so, or says it could not. */
  async function copyMessage(text: string, supplierId: string | null): Promise<void> {
    copyStatus.value = { supplierId, text: (await copyText(text)) ? 'Mensagem copiada.' : 'Não foi possível copiar a mensagem.' }
  }

  // ---- Enviadas ----

  const orders = shallowRef<SupplierOrderSummary[]>([])
  const ordersFilter = ref<'open' | 'all'>('open')
  const ordersCursor = ref<string | undefined>(undefined)
  const ordersLoading = ref(false)
  const ordersLoaded = ref(false)
  const ordersError = ref<string | null>(null)
  let ordersRead = 0

  async function readOrders(cursor: string | undefined): Promise<void> {
    const read = ++ordersRead
    ordersLoading.value = true
    ordersError.value = null
    try {
      const page = await supplierOrders(api, { state: ordersFilter.value, cursor, limit: pageSize })
      // A newer read (another filter) started meanwhile: its answer is the one to show.
      if (read !== ordersRead) return
      orders.value = cursor === undefined ? page.items : [...orders.value, ...page.items]
      ordersCursor.value = page.nextCursor
      ordersLoaded.value = true
    } catch (failure) {
      if (read === ordersRead) ordersError.value = errorMessage(failureOf(failure))
    } finally {
      if (read === ordersRead) ordersLoading.value = false
    }
  }

  /** loadOrders reads the first page of "Enviadas" with the filter set, "Abertos" or "Todos". */
  async function loadOrders(filter: 'open' | 'all' = ordersFilter.value): Promise<void> {
    ordersFilter.value = filter
    await readOrders(undefined)
  }

  /** loadMoreOrders appends the next page ("Carregar mais"). */
  async function loadMoreOrders(): Promise<void> {
    if (ordersCursor.value !== undefined) await readOrders(ordersCursor.value)
  }

  // ---- Avisar o fornecedor ----

  const notices = shallowRef<Notice[]>([])
  const noticeBusy = ref<string | null>(null)
  const noticeStatus = ref<string | null>(null)
  const noticeError = ref<string | null>(null)

  /** loadNotices reads the sent lines that are no longer needed. A failed read leaves the old list and says nothing: Compras still works without it. */
  async function loadNotices(): Promise<void> {
    try {
      notices.value = (await readNotices(api)).items
    } catch (failure) {
      failureOf(failure)
    }
  }

  /** resolve cancels a notice's line ("Marcar como resolvido") and says so; a line already closed leaves the list as it is now. */
  async function resolve(lineId: string): Promise<boolean> {
    noticeBusy.value = lineId
    noticeStatus.value = null
    noticeError.value = null
    try {
      await resolveNotice(api, lineId)
      noticeStatus.value = 'Resolvido. A linha foi cancelada.'
      void readToBuy()
      return true
    } catch (failure) {
      noticeError.value = errorMessage(failureOf(failure), { line_closed: lineClosedMessage })
      return false
    } finally {
      noticeBusy.value = null
      await loadNotices()
    }
  }

  /** openNotice opens the wa.me link that asks the supplier to cancel ("Avisar pelo WhatsApp"). */
  function openNotice(notice: Notice): void {
    openWhatsapp(notice.whatsappUrl)
  }

  // ---- Pedido de compra ----

  const detail = shallowRef<SupplierOrder | null>(null)
  const detailLoading = ref(false)
  const detailError = ref<string | null>(null)
  const busy = ref(false)
  const notice = ref<string | null>(null)
  /** error is the refusal of an action done from the page; `dialogError` is that of an action done in a dialog. */
  const error = ref<string | null>(null)
  const dialogError = ref<string | null>(null)
  const conflict = ref(false)
  const delayWarnings = shallowRef<DelayWarning[]>([])
  let detailRead = 0

  function resetDetail(): void {
    detail.value = null
    detailError.value = null
    notice.value = null
    error.value = null
    dialogError.value = null
    conflict.value = false
    delayWarnings.value = []
    detailRead += 1
  }

  /** openOrder reads one pedido de compra and the store's time zone. */
  async function openOrder(supplierOrderId: string): Promise<void> {
    resetDetail()
    const mine = detailRead
    detailLoading.value = true
    try {
      const [read] = await Promise.all([readSupplierOrder(api, supplierOrderId), timeZone.value === undefined ? readSettings() : undefined])
      if (mine === detailRead) detail.value = read
    } catch (failure) {
      if (mine === detailRead) detailError.value = errorMessage(failureOf(failure), { not_found: 'Pedido de compra não encontrado.' })
    } finally {
      if (mine === detailRead) detailLoading.value = false
    }
  }

  /** reloadOrder reads the open pedido de compra again ("Recarregar"), dropping every message. */
  async function reloadOrder(): Promise<void> {
    if (detail.value !== null) await openOrder(detail.value.id)
  }

  function currentOrder(): SupplierOrder {
    if (detail.value === null) throw new Error('no pedido de compra is open')
    return detail.value
  }

  /**
   * act runs a write on the open pedido de compra and, on success, takes the
   * answer it carries. A conflict always shows on the page with "Recarregar",
   * since every retry would send the same stale version.
   */
  async function act(write: (current: SupplierOrder) => Promise<SupplierOrder>, options: OrderWrite): Promise<boolean> {
    const current = currentOrder()
    busy.value = true
    notice.value = null
    error.value = null
    dialogError.value = null
    conflict.value = false
    delayWarnings.value = []
    try {
      detail.value = await write(current)
      notice.value = options.done
      return true
    } catch (failure) {
      const refused = failureOf(failure)
      conflict.value = refused.code === 'version_conflict'
      const message = errorMessage(refused, { version_conflict: conflictMessage, line_closed: lineClosedMessage, ...options.own })
      if (options.inDialog === true && !conflict.value) dialogError.value = message
      else error.value = message
      return false
    } finally {
      busy.value = false
    }
  }

  /** savePromisedDate records the supplier's promised date, with a different date for some lines. */
  async function savePromisedDate(date: string, lines: { lineId: string; date: string }[]): Promise<boolean> {
    const saved = await act(
      (current) => recordPromisedDate(api, current.id, { version: current.version, date, ...(lines.length === 0 ? {} : { lines }) }),
      {
        inDialog: true,
        own: { invalid: 'Escolha uma data a partir de hoje.' },
        done: 'Prazo salvo. As datas previstas dos pedidos foram atualizadas.',
      },
    )
    if (saved) delayWarnings.value = lateLines(currentOrder())
    return saved
  }

  /** lateLines are the open lines the new dates left with an alert: "{item} do pedido P-0231 pode atrasar a entrega." */
  function lateLines(order: SupplierOrder): DelayWarning[] {
    return order.lines
      .filter((line) => (line.status === 'sent' || line.status === 'in_transit') && line.alerts.length > 0)
      .map((line) => ({ lineId: line.id, itemName: line.name, orderId: line.orderId, orderNumber: line.orderNumber }))
  }

  /** markOrderInTransit marks every sent line as in transit. */
  function markOrderInTransit(): Promise<boolean> {
    return act((current) => markInTransit(api, current.id, { version: current.version }), { done: 'Itens em trânsito.' })
  }

  /** undo undoes the send: its lines are cancelled and its items go back to "A comprar". */
  function undo(): Promise<boolean> {
    return act((current) => undoSupplierOrder(api, current.id, { version: current.version }), {
      inDialog: true,
      own: { supplier_order_received: receivedMessage },
      done: 'Envio desfeito.',
    })
  }

  /** savePaidPrices records the unit price paid per line, in cents (owner only). */
  function savePaidPrices(prices: { lineId: string; cents: number }[]): Promise<boolean> {
    return act((current) => recordPaidPrices(api, current.id, { version: current.version, prices }), {
      inDialog: true,
      own: { invalid: 'Confira o valor.' },
      done: 'Preços salvos. As margens do Financeiro usam o preço pago.',
    })
  }

  /** openChat opens the chat with the supplier with the link the API sent on the pedido de compra. */
  function openChat(): void {
    if (detail.value !== null) openWhatsapp(detail.value.whatsappUrl)
  }

  /** clearDialogError forgets the refusal shown in a dialog, when one is opened. */
  function clearDialogError(): void {
    dialogError.value = null
  }

  // ---- Fornecedores ----

  const directory = shallowRef<Supplier[]>([])
  const directoryCursor = ref<string | undefined>(undefined)
  const directoryLoading = ref(false)
  const directoryLoaded = ref(false)
  const directoryError = ref<string | null>(null)
  const query = ref('')
  const supplierNotice = ref<string | null>(null)
  let directoryRead = 0

  /** editing is the supplier in the drawer, or null for a new one; `drawerOpen` says whether the drawer is there at all. */
  const editing = shallowRef<Supplier | null>(null)
  const drawerOpen = ref(false)
  /** drawerKey changes when the drawer's supplier is read again, so the form starts over from it. */
  const drawerKey = ref(0)
  const supplierConflict = ref(false)
  const supplierItemList = shallowRef<SupplierItemUse[]>([])
  const supplierItemsLoading = ref(false)
  const supplierItemsError = ref<string | null>(null)
  const supplierForm = useForm()

  async function readDirectory(cursor: string | undefined): Promise<void> {
    const read = ++directoryRead
    directoryLoading.value = true
    directoryError.value = null
    try {
      const text = query.value.trim()
      const page = await listSuppliers(api, { cursor, limit: supplierPageSize, q: text === '' ? undefined : text })
      if (read !== directoryRead) return
      directory.value = cursor === undefined ? page.items : [...directory.value, ...page.items]
      directoryCursor.value = page.nextCursor
      directoryLoaded.value = true
    } catch (failure) {
      if (read === directoryRead) directoryError.value = errorMessage(failureOf(failure))
    } finally {
      if (read === directoryRead) directoryLoading.value = false
    }
  }

  /** loadSuppliers reads the first page of Fornecedores with the search text. */
  function loadSuppliers(): Promise<void> {
    return readDirectory(undefined)
  }

  /** loadMoreSuppliers appends the next page. */
  async function loadMoreSuppliers(): Promise<void> {
    if (directoryCursor.value !== undefined) await readDirectory(directoryCursor.value)
  }

  async function readSupplierItems(supplierId: string): Promise<void> {
    supplierItemsLoading.value = true
    supplierItemsError.value = null
    try {
      supplierItemList.value = (await supplierItems(api, supplierId)).items
    } catch (failure) {
      supplierItemsError.value = errorMessage(failureOf(failure))
    } finally {
      supplierItemsLoading.value = false
    }
  }

  function showSupplier(supplier: Supplier | null): void {
    editing.value = supplier
    supplierItemList.value = []
    supplierItemsError.value = null
    supplierForm.clear()
    supplierConflict.value = false
    drawerKey.value += 1
    drawerOpen.value = true
    if (supplier !== null) void readSupplierItems(supplier.id)
  }

  /** openSupplier opens the drawer on a supplier, or on a new one with null, and reads the items the supplier supplies. */
  function openSupplier(supplier: Supplier | null): void {
    supplierNotice.value = null
    showSupplier(supplier)
  }

  /** closeSupplier closes the drawer, dropping unsaved edits. */
  function closeSupplier(): void {
    drawerOpen.value = false
    editing.value = null
    supplierItemList.value = []
    supplierForm.clear()
    supplierConflict.value = false
  }

  /** reloadSupplier reads the open supplier again ("Recarregar"), dropping the edits that lost the race. */
  async function reloadSupplier(): Promise<void> {
    const current = editing.value
    if (current === null) return
    try {
      showSupplier(await getSupplier(api, current.id))
    } catch (failure) {
      supplierForm.error.value = errorMessage(failureOf(failure))
    }
  }

  /** saveSupplier creates or replaces the supplier from the form and resolves whether it was saved. The number is typed with its DDD and sent as digits. */
  async function saveSupplier(name: string, whatsapp: string): Promise<boolean> {
    supplierConflict.value = false
    const current = editing.value
    const saved = await supplierForm.attempt(
      supplierChecks(name, whatsapp),
      { byCode: { version_conflict: supplierConflictMessage, invalid: fixTheFields } },
      async () => {
        const body = { name: name.trim(), whatsappPhone: whatsappDigits(whatsapp) }
        try {
          if (current === null) await createSupplier(api, body)
          else await replaceSupplier(api, current.id, { ...body, version: current.version })
        } catch (failure) {
          supplierConflict.value = failure instanceof ApiError && failure.code === 'version_conflict'
          throw failure
        }
      },
    )
    if (!saved) return false
    supplierNotice.value = 'Fornecedor salvo.'
    closeSupplier()
    await loadSuppliers()
    return true
  }

  // ---- Tabs ----

  /** openTab reads what a tab shows, every time it is shown: Compras reads the orders afresh, so a send from another device is seen. */
  async function openTab(tab: ComprasTab): Promise<void> {
    if (tab === 'a-comprar') await loadToBuy()
    else if (tab === 'enviadas') await loadOrders()
    else if (tab === 'fornecedores') await loadSuppliers()
  }

  return {
    timeZone,
    purchaseBeforeDeposit,
    sendsCosts,
    suppliers,
    truncated,
    toBuyLoading,
    toBuyLoaded,
    toBuyError,
    preview,
    sending,
    sent,
    copyStatus,
    cardError,
    unchecked,
    loadToBuy,
    setPicked,
    togglePreview,
    send,
    openSentLink,
    copyMessage,
    orders,
    ordersFilter,
    hasMoreOrders: computed(() => ordersCursor.value !== undefined),
    ordersLoading,
    ordersLoaded,
    ordersError,
    loadOrders,
    loadMoreOrders,
    notices,
    noticeBusy,
    noticeStatus,
    noticeError,
    loadNotices,
    resolve,
    openNotice,
    detail,
    detailLoading,
    detailError,
    busy,
    notice,
    error,
    dialogError,
    conflict,
    delayWarnings,
    openOrder,
    reloadOrder,
    savePromisedDate,
    markOrderInTransit,
    undo,
    savePaidPrices,
    openChat,
    clearDialogError,
    directory,
    hasMoreSuppliers: computed(() => directoryCursor.value !== undefined),
    directoryLoading,
    directoryLoaded,
    directoryError,
    query,
    supplierNotice,
    editing,
    drawerOpen,
    drawerKey,
    supplierConflict,
    supplierItemList,
    supplierItemsLoading,
    supplierItemsError,
    savingSupplier: supplierForm.submitting,
    supplierFormError: supplierForm.error,
    supplierFieldErrors: supplierForm.fieldErrors,
    loadSuppliers,
    loadMoreSuppliers,
    openSupplier,
    closeSupplier,
    reloadSupplier,
    saveSupplier,
    openTab,
  }
})
