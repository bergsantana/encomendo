import {
  ApiError,
  changeOrder,
  type ChangedOrder,
  createCustomer,
  createOrder,
  type CreatedOrder,
  type Customer,
  type CustomerSummary,
  getCustomer,
  getItem,
  getKit,
  getOrder,
  getSettings,
  type Item,
  type ItemSummary,
  type Kit,
  type KitSummary,
  listCustomers,
  listItems,
  listKits,
  newIdempotencyKey,
  type Order,
  type Quote,
  quoteOrder,
  sendTrackingLink,
  type TenantSettings,
  updateCustomer,
} from '@encomendo/api'
import { defineStore } from 'pinia'
import { computed, reactive, ref, shallowRef, watch } from 'vue'

import { api } from '../../api'
import { type FieldCheck, useForm } from '../../composables/useForm'
import { errorMessage, issuesByField } from '../../errorMessage'
import { useSessionStore } from '../../stores/session'
import { type AddressDraft, customerChecks, draftFromCustomer, emptyDraft, toCustomerInput } from '../clientes/customerForm'
import { searchTerm } from '../clientes/store'
import { openWhatsapp } from './openLink'
import {
  type CatalogOption,
  effectiveItems,
  emptyOrderDraft,
  type LineDraft,
  newAddressChoice,
  optionKey,
  type OrderDraft,
  termsChecks,
  termsFromOrder,
  toLineRequest,
  toTerms,
  wholeQuantity,
} from './orderForm'

const quoteDelayMs = 300
const searchSize = 20
const conflictMessage = 'Outra pessoa alterou este pedido. Recarregue para ver a versão atual.'
const fixTheFields = 'Confira os campos destacados.'

/** termsFieldOf maps the field of an API error to the form field it is about, or null for one the form has no place for. */
function termsFieldOf(apiField: string): string | null {
  const name = apiField.toLowerCase()
  const known: [string, string][] = [
    ['discount', 'discount'],
    ['freight', 'freight'],
    ['address', 'address'],
    ['deposit', 'depositPercent'],
    ['assembly', 'assemblyDays'],
    ['safety', 'safetyMarginDays'],
  ]
  return known.find(([part]) => name.includes(part))?.[1] ?? null
}

/** MissingOption is a required option of a line that has no choice yet. */
export interface MissingOption {
  field: string
  name: string
}

/**
 * useOrderFormStore backs "Novo pedido" and "Alterar pedido": the customer,
 * the lines with their swaps and options, the terms, and the quote. Totals,
 * deposit and dates are never added up here: `POST /orders/quote` prices the
 * form 300 ms after the last edit and only its newest answer is kept. One
 * `Idempotency-Key` is made per save and kept for its retries; it is made
 * again only when the body changes (ADR 0030).
 */
export const useOrderFormStore = defineStore('orderForm', () => {
  const session = useSessionStore()

  const order = shallowRef<Order | null>(null)
  const loading = ref(false)
  const loadError = ref<string | null>(null)
  const settings = shallowRef<TenantSettings | null>(null)
  const customer = shallowRef<Customer | null>(null)
  const draft = reactive<OrderDraft>(emptyOrderDraft())
  const kits = shallowRef<Record<string, Kit>>({})
  const items = shallowRef<Record<string, Item>>({})
  /** archivedNames are the items or kits the last answer named as archived (`item_unavailable`). */
  const archivedNames = ref<string[]>([])

  const quote = shallowRef<Quote | null>(null)
  const quoting = ref(false)
  const quoteError = ref<string | null>(null)
  const quoteIssues = ref<Record<string, string>>({})

  const form = useForm()
  const conflict = ref(false)
  const created = shallowRef<CreatedOrder | null>(null)
  const changed = shallowRef<ChangedOrder | null>(null)
  const notice = ref<string | null>(null)

  const customerResults = shallowRef<CustomerSummary[]>([])
  const customerSearching = ref(false)
  const customerForm = useForm()
  const addressForm = useForm()

  const catalogResults = shallowRef<{ kits: KitSummary[]; items: ItemSummary[] }>({ kits: [], items: [] })
  const catalogSearching = ref(false)
  const catalogError = ref<string | null>(null)

  let saveKey: string | null = null
  let saveBody = ''
  let initial = ''
  let timer: ReturnType<typeof setTimeout> | undefined
  let latest = 0
  let controller: AbortController | undefined
  let searchRead = 0
  let catalogRead = 0

  const mode = computed<'new' | 'change'>(() => (order.value === null ? 'new' : 'change'))
  const canManage = computed(() => session.can('order:manage'))
  const canWriteCustomer = computed(() => session.can('customer:write'))

  /** needsApproval is true while the order being changed was paid: the seller confirms the customer agreed. */
  const needsApproval = computed(() => order.value?.editable === 'approval')

  // ---- Quote ----

  /** missingOptions lists the required options of a line that have no choice. */
  function missingOptions(line: LineDraft): MissingOption[] {
    const missing: MissingOption[] = []
    const kit = kits.value[line.catalogId]
    for (const itemId of effectiveItems(line, kit)) {
      for (const option of items.value[itemId]?.options ?? []) {
        if (option.required && line.choices[optionKey(itemId, option.id)] === undefined) {
          missing.push({ field: `option.${line.key}.${itemId}.${option.id}`, name: option.name })
        }
      }
    }
    return missing
  }

  const access = computed(() => ({ canManage: canManage.value }))

  /**
   * fieldErrors are the messages next to the fields: what the last save
   * found wrong, and what the quote's refusal said about a field of "Entrega"
   * or "Valores" (the API names them `terms.discountCents` and the like).
   */
  const fieldErrors = computed<Record<string, string>>(() => {
    const fromQuote: Record<string, string> = {}
    for (const [apiField, message] of Object.entries(quoteIssues.value)) {
      const field = termsFieldOf(apiField)
      if (field !== null) fromQuote[field] ??= message
    }
    return { ...fromQuote, ...form.fieldErrors.value }
  })

  /**
   * quoteBody is what the quote is asked for, or null while the form cannot
   * be priced yet: no lines, a quantity or value that is not a number, a
   * required option without a choice, or the details of an item still loading.
   */
  const quoteBody = computed(() => {
    if (draft.lines.length === 0) return null
    if (draft.lines.some((line) => wholeQuantity(line.quantity) === null || missingOptions(line).length > 0)) return null
    if (draft.lines.some((line) => line.kind === 'kit' && kits.value[line.catalogId] === undefined)) return null
    if (termsChecks(draft.terms, access.value).some(([, value, , isValid = (text: string) => text !== '']) => !isValid(value))) return null
    return {
      lines: draft.lines.map(toLineRequest),
      terms: toTerms(draft.terms, access.value),
      ...(order.value === null ? {} : { orderId: order.value.id }),
    }
  })

  async function runQuote(body: NonNullable<typeof quoteBody.value>): Promise<void> {
    const mine = ++latest
    controller?.abort()
    controller = new AbortController()
    try {
      const answer = await quoteOrder(api, body, { signal: controller.signal })
      if (mine !== latest) return
      quote.value = answer
      quoteError.value = null
      quoteIssues.value = {}
      archivedNames.value = []
    } catch (failure) {
      if (!(failure instanceof ApiError)) throw failure
      // A newer edit is already being priced: this answer is stale, whatever it says.
      if (mine !== latest) return
      quote.value = null
      quoteIssues.value = issuesByField(failure)
      quoteError.value = errorMessage(failure, { item_unavailable: unavailableMessage(failure) })
      archivedNames.value = failure.code === 'item_unavailable' ? failure.references.map((reference) => reference.name) : []
    } finally {
      if (mine === latest) quoting.value = false
    }
  }

  watch(
    () => JSON.stringify(quoteBody.value),
    () => {
      clearTimeout(timer)
      latest += 1
      controller?.abort()
      const body = quoteBody.value
      quote.value = null
      quoteError.value = null
      quoteIssues.value = {}
      quoting.value = body !== null
      if (body !== null) timer = setTimeout(() => void runQuote(body), quoteDelayMs)
    },
  )

  // ---- Catalog ----

  async function ensureItems(ids: readonly string[]): Promise<void> {
    const missing = [...new Set(ids)].filter((id) => items.value[id] === undefined)
    const read = await Promise.all(missing.map((id) => getItem(api, id)))
    items.value = { ...items.value, ...Object.fromEntries(read.map((item) => [item.id, item])) }
  }

  async function ensureKit(id: string): Promise<Kit> {
    const known = kits.value[id]
    if (known !== undefined) return known
    const kit = await getKit(api, id)
    await ensureItems(kit.items.flatMap((item) => [item.itemId, ...item.alternativeItemIds]))
    kits.value = { ...kits.value, [id]: kit }
    return kit
  }

  /** searchCatalog reads the kits or the items that match a name, for "Adicionar kit" and "Adicionar item". */
  async function searchCatalog(what: 'kits' | 'items', text: string): Promise<void> {
    const read = ++catalogRead
    catalogSearching.value = true
    catalogError.value = null
    const q = text.trim() === '' ? undefined : text.trim()
    try {
      const found = what === 'kits' ? (await listKits(api, { q, limit: searchSize })).items : (await listItems(api, { q, limit: searchSize })).items
      if (read !== catalogRead) return
      catalogResults.value = what === 'kits' ? { kits: found as KitSummary[], items: [] } : { kits: [], items: found as ItemSummary[] }
    } catch (failure) {
      if (!(failure instanceof ApiError)) throw failure
      if (read === catalogRead) catalogError.value = errorMessage(failure)
    } finally {
      if (read === catalogRead) catalogSearching.value = false
    }
  }

  let lineCount = 0
  function nextKey(): string {
    lineCount += 1
    return `line-${lineCount}`
  }

  /** addKit puts a kit in the order, with its items and alternatives read for the swaps and options. */
  async function addKit(kitId: string): Promise<boolean> {
    try {
      const kit = await ensureKit(kitId)
      draft.lines.push({ key: nextKey(), kind: 'kit', catalogId: kit.id, name: kit.name, quantity: '1', swaps: {}, choices: {} })
      return true
    } catch (failure) {
      if (!(failure instanceof ApiError)) throw failure
      catalogError.value = errorMessage(failure)
      return false
    }
  }

  /** addItem puts a loose item in the order, with its options read. */
  async function addItem(itemId: string): Promise<boolean> {
    try {
      await ensureItems([itemId])
      const item = items.value[itemId]
      if (item === undefined) return false
      draft.lines.push({ key: nextKey(), kind: 'item', catalogId: item.id, name: item.name, quantity: '1', swaps: {}, choices: {} })
      return true
    } catch (failure) {
      if (!(failure instanceof ApiError)) throw failure
      catalogError.value = errorMessage(failure)
      return false
    }
  }

  function removeLine(key: string): void {
    const index = draft.lines.findIndex((line) => line.key === key)
    if (index >= 0) draft.lines.splice(index, 1)
  }

  /** swap puts `withItemId` in the place of a kit's item, or the original back when it is ''. The options of the item that left are forgotten. */
  function swap(key: string, itemId: string, withItemId: string): void {
    const line = draft.lines.find((candidate) => candidate.key === key)
    if (line === undefined) return
    const leaving = line.swaps[itemId] ?? itemId
    line.choices = Object.fromEntries(Object.entries(line.choices).filter(([choice]) => !choice.startsWith(`${leaving}:`)))
    const kept = Object.entries(line.swaps).filter(([original]) => original !== itemId)
    line.swaps = Object.fromEntries(withItemId === '' || withItemId === itemId ? kept : [...kept, [itemId, withItemId]])
  }

  /** choose records the client's choice of an option, or clears it when `choiceId` is ''. */
  function choose(key: string, itemId: string, option: CatalogOption, choiceId: string): void {
    const line = draft.lines.find((candidate) => candidate.key === key)
    if (line === undefined) return
    const chosenKey = optionKey(itemId, option.id)
    const kept = Object.entries(line.choices).filter(([existing]) => existing !== chosenKey)
    line.choices = Object.fromEntries(choiceId === '' ? kept : [...kept, [chosenKey, choiceId]])
  }

  // ---- Customer ----

  /** searchCustomers reads the customers matching "Buscar cliente" (a name or a WhatsApp number). */
  async function searchCustomers(text: string): Promise<void> {
    const read = ++searchRead
    customerSearching.value = true
    try {
      const page = await listCustomers(api, { q: searchTerm(text), limit: searchSize })
      if (read === searchRead) customerResults.value = page.items
    } catch (failure) {
      if (!(failure instanceof ApiError)) throw failure
      if (read === searchRead) customerForm.error.value = errorMessage(failure)
    } finally {
      if (read === searchRead) customerSearching.value = false
    }
  }

  function useCustomer(chosen: Customer): void {
    customer.value = chosen
    draft.terms.addressId = ''
    customerResults.value = []
    customerForm.clear()
  }

  /** chooseCustomer reads the customer with the addresses and puts it on the order. */
  async function chooseCustomer(customerId: string): Promise<void> {
    try {
      useCustomer(await getCustomer(api, customerId))
    } catch (failure) {
      if (!(failure instanceof ApiError)) throw failure
      customerForm.error.value = errorMessage(failure)
    }
  }

  /** clearCustomer takes the customer off the order to choose another. */
  function clearCustomer(): void {
    customer.value = null
    draft.terms.addressId = ''
  }

  /**
   * createCustomerInline makes the customer of "Novo cliente" (Nome and
   * WhatsApp) with the Clientes call, so the order can name an address of its
   * own customer. `order:write` is enough for it; a number another customer
   * has is `phone_taken`.
   */
  async function createCustomerInline(name: string, phone: string): Promise<boolean> {
    const typed = { ...emptyDraft(), name, phone }
    return customerForm.attempt(
      customerChecks(typed),
      { byCode: { phone_taken: fixTheFields, invalid: fixTheFields }, fieldByCode: { phone_taken: ['phone', 'Já existe um cliente com este WhatsApp.'] } },
      async () => useCustomer(await createCustomer(api, toCustomerInput(typed))),
    )
  }

  /**
   * saveAddress saves a new address on the order's customer (a delivery needs
   * one of the customer's addresses) and chooses it. The API has no address
   * in an order, so this is the customer's own update, which needs
   * `customer:write`.
   */
  async function saveAddress(address: AddressDraft): Promise<boolean> {
    const current = customer.value
    if (current === null) return false
    const typed = draftFromCustomer(current)
    typed.addresses.push(address)
    return addressForm.attempt(
      // Checked on its own, so its fields are `addresses[0].…` as AddressFields reads them.
      customerChecks({ ...emptyDraft(), addresses: [address] }).filter(([field]) => field.startsWith('addresses[')),
      { byCode: { version_conflict: 'Outra pessoa alterou este cliente. Recarregue para ver a versão atual.' } },
      async () => {
        const saved = await updateCustomer(api, current.id, { ...toCustomerInput(typed), version: current.version })
        const known = new Set(current.addresses.map((existing) => existing.id))
        const made = saved.addresses.find((existing) => !known.has(existing.id))
        customer.value = saved
        draft.terms.addressId = made?.id ?? ''
      },
    )
  }

  // ---- Starting ----

  function reset(): void {
    clearTimeout(timer)
    latest += 1
    controller?.abort()
    order.value = null
    customer.value = null
    Object.assign(draft, emptyOrderDraft())
    quote.value = null
    quoting.value = false
    quoteError.value = null
    quoteIssues.value = {}
    archivedNames.value = []
    form.clear()
    customerForm.clear()
    addressForm.clear()
    conflict.value = false
    created.value = null
    changed.value = null
    notice.value = null
    loadError.value = null
    customerResults.value = []
    catalogResults.value = { kits: [], items: [] }
    saveKey = null
    saveBody = ''
  }

  async function readSettings(): Promise<void> {
    try {
      settings.value = await getSettings(api)
    } catch (failure) {
      if (!(failure instanceof ApiError)) throw failure
      // The defaults are only hints; the form works without them.
    }
  }

  function snapshot(): string {
    return JSON.stringify({ customer: customer.value?.id ?? null, lines: draft.lines.map((line) => [line.kind, line.catalogId, line.quantity, line.swaps, line.choices]), terms: draft.terms, approved: draft.customerApproved })
  }

  /** stop cancels the quote still waiting or in flight, for when the form is left. */
  function stop(): void {
    clearTimeout(timer)
    latest += 1
    controller?.abort()
    quoting.value = false
  }

  /** startNew opens an empty "Novo pedido". */
  async function startNew(): Promise<void> {
    reset()
    await readSettings()
    initial = snapshot()
  }

  /**
   * lineFromOrder rebuilds a line of the order for the form: the kit with its
   * swaps and quantity, or the loose item, with the options the client chose.
   */
  function lineFromOrder(source: Order, index: number): LineDraft | null {
    const line = source.lines[index]
    if (line === undefined) return null
    const lineItems = source.items.filter((item) => item.lineId === line.id)
    const choices: Record<string, string> = {}
    for (const item of lineItems) {
      for (const chosen of item.options) choices[optionKey(item.catalogItemId, chosen.optionId)] = chosen.choiceId
    }
    if (line.kit === null) {
      const only = lineItems[0]
      if (only === undefined) return null
      return { key: nextKey(), kind: 'item', catalogId: only.catalogItemId, name: only.name, quantity: String(only.quantity), swaps: {}, choices }
    }
    const swaps = Object.fromEntries(line.kit.swaps.map((swapped) => [swapped.removedItemId, swapped.addedItemId]))
    return { key: nextKey(), kind: 'kit', catalogId: line.kit.id, name: line.kit.name, quantity: String(kitQuantity(lineItems, kits.value[line.kit.id], swaps)), swaps, choices }
  }

  /**
   * kitQuantity is how many kits a line holds, from its first item: the
   * order item's quantity is the kit's quantity of it times the line's. The
   * order does not store the line's quantity, and sending 1 for a line of two
   * would reprice the order.
   */
  function kitQuantity(lineItems: Order['items'], kit: Kit | undefined, swaps: Record<string, string>): number {
    const first = kit?.items[0]
    if (first === undefined) return 1
    const held = lineItems.find((item) => item.catalogItemId === (swaps[first.itemId] ?? first.itemId))
    if (held === undefined || first.quantity < 1 || held.quantity % first.quantity !== 0) return 1
    return held.quantity / first.quantity
  }

  /** startChange reads the order and fills the form with it ("Alterar pedido"). */
  async function startChange(orderId: string): Promise<void> {
    reset()
    loading.value = true
    try {
      const [read] = await Promise.all([getOrder(api, orderId), readSettings()])
      const kitIds = read.lines.flatMap((line) => (line.kit === null ? [] : [line.kit.id]))
      await Promise.all(kitIds.map(ensureKit))
      await ensureItems(read.items.map((item) => item.catalogItemId))
      const found = await getCustomer(api, read.customer.id)
      order.value = read
      customer.value = found
      draft.lines = read.lines.flatMap((_line, index) => lineFromOrder(read, index) ?? [])
      draft.terms = termsFromOrder(read)
      initial = snapshot()
    } catch (failure) {
      if (!(failure instanceof ApiError)) throw failure
      loadError.value = errorMessage(failure, { not_found: 'Pedido não encontrado.' })
    } finally {
      loading.value = false
    }
  }

  /** dirty is true when the form holds edits that were not saved, so leaving asks "Descartar as alterações?". */
  const dirty = computed(() => created.value === null && changed.value === null && snapshot() !== initial)

  // ---- Saving ----

  function unavailableMessage(failure: ApiError): string {
    const names = failure.references.map((reference) => reference.name)
    if (names.length === 0) return 'Um item ou kit do pedido está arquivado.'
    return names.map((name) => `${name} está arquivado e não pode entrar no pedido.`).join(' ')
  }

  function saveChecks(): FieldCheck[] {
    const checks: FieldCheck[] = [['lines', String(draft.lines.length), 'Adicione ao menos um item ou kit.', (count) => count !== '0']]
    if (order.value === null) checks.unshift(['customer', customer.value?.id ?? '', 'Informe o cliente do pedido.'])
    for (const line of draft.lines) {
      checks.push([`quantity.${line.key}`, line.quantity, 'Use uma quantidade de 1 a 999.', (text) => wholeQuantity(text) !== null])
      for (const missing of missingOptions(line)) checks.push([missing.field, '', `Escolha ${missing.name}.`])
    }
    const address = draft.terms.handover === 'delivery' && draft.terms.addressId === newAddressChoice ? '' : draft.terms.addressId
    checks.push(['address', draft.terms.handover === 'delivery' ? address : 'pickup', 'Escolha o endereço de entrega.'])
    if (needsApproval.value) checks.push(['approval', draft.customerApproved ? 'yes' : '', 'Confirme que o cliente aprovou a mudança.'])
    return [...checks, ...termsChecks(draft.terms, access.value)]
  }

  /** keyFor makes the key of this save, and keeps it while the body stays the same, so a retry is answered with the first result. */
  function keyFor(body: unknown): string {
    const text = JSON.stringify(body)
    if (saveKey === null || text !== saveBody) {
      saveKey = newIdempotencyKey()
      saveBody = text
    }
    return saveKey
  }

  async function submit(sendToCustomer: boolean): Promise<void> {
    const lines = draft.lines.map(toLineRequest)
    const terms = toTerms(draft.terms, access.value)
    const current = order.value
    if (current !== null) {
      changed.value = await changeOrder(api, current.id, { lines, terms, customerApproved: draft.customerApproved, version: current.version })
      return
    }
    const body = { customerId: customer.value?.id, lines, terms, sendToCustomer }
    created.value = await createOrder(api, body, keyFor(body))
    saveKey = null
  }

  /**
   * save creates the order, or saves the change, and resolves whether it
   * worked. With `sendToCustomer` the order goes to its next status and the
   * answer's wa.me link opens. A refusal stays on the form with its message;
   * the idempotency key stays too, so pressing "Salvar" again is a retry.
   */
  async function save(sendToCustomer: boolean): Promise<boolean> {
    conflict.value = false
    archivedNames.value = []
    const saved = await form.attempt(
      saveChecks(),
      { byCode: { version_conflict: conflictMessage, order_not_editable: 'Este pedido já está em montagem e não pode mais ser alterado.' } },
      async () => {
        try {
          await submit(sendToCustomer)
        } catch (failure) {
          if (failure instanceof ApiError) onSaveRefused(failure)
          throw failure
        }
      },
    )
    if (!saved) {
      if (archivedNames.value.length > 0) form.error.value = archivedNames.value.map((name) => `${name} está arquivado e não pode entrar no pedido.`).join(' ')
      return false
    }
    const url = created.value?.whatsappUrl
    if (sendToCustomer && url !== undefined) openWhatsapp(url)
    return true
  }

  function onSaveRefused(failure: ApiError): void {
    conflict.value = failure.code === 'version_conflict'
    if (failure.code === 'item_unavailable') archivedNames.value = failure.references.map((reference) => reference.name)
    // The same key with another body is a new action: the next try makes a new key.
    if (failure.code === 'idempotency_key_reused') saveKey = null
  }

  /** reload reads the order again after a version conflict, keeping nothing of the lost edits. */
  async function reload(): Promise<void> {
    const current = order.value
    if (current !== null) await startChange(current.id)
  }

  /** sendTracking opens WhatsApp with the tracking link of the order just created; a draft saved without sending asks for a new link first. */
  async function sendTracking(): Promise<void> {
    const made = created.value
    if (made === null) return
    if (made.whatsappUrl !== undefined) {
      openWhatsapp(made.whatsappUrl)
      return
    }
    try {
      const link = await sendTrackingLink(api, made.order.id)
      created.value = { order: made.order, trackingUrl: link.trackingUrl, whatsappUrl: link.whatsappUrl }
      openWhatsapp(link.whatsappUrl)
    } catch (failure) {
      if (!(failure instanceof ApiError)) throw failure
      notice.value = errorMessage(failure)
    }
  }

  /** openChangeMessage opens the wa.me link that tells the customer about a paid order's change. */
  function openChangeMessage(): void {
    const url = changed.value?.whatsappUrl
    if (url !== undefined) openWhatsapp(url)
  }

  return {
    order,
    mode,
    loading,
    loadError,
    settings,
    customer,
    draft,
    kits,
    items,
    quote,
    quoting,
    quoteError,
    quoteIssues,
    archivedNames,
    needsApproval,
    canManage,
    canWriteCustomer,
    saving: form.submitting,
    formError: form.error,
    fieldErrors,
    conflict,
    created,
    changed,
    notice,
    dirty,
    customerResults,
    customerSearching,
    customerError: customerForm.error,
    customerFieldErrors: customerForm.fieldErrors,
    customerSaving: customerForm.submitting,
    addressError: addressForm.error,
    addressFieldErrors: addressForm.fieldErrors,
    addressSaving: addressForm.submitting,
    catalogResults,
    catalogSearching,
    catalogError,
    missingOptions,
    startNew,
    startChange,
    stop,
    reload,
    searchCatalog,
    addKit,
    addItem,
    removeLine,
    swap,
    choose,
    searchCustomers,
    chooseCustomer,
    clearCustomer,
    createCustomerInline,
    saveAddress,
    save,
    sendTracking,
    openChangeMessage,
  }
})
