import { ApiError, createCustomer, type Customer, type CustomerSummary, getCustomer, getSettings, listCustomers, listOrders, type OrderSummary, updateCustomer } from '@encomendo/api'
import { defineStore } from 'pinia'
import { computed, ref, shallowRef } from 'vue'

import { api } from '../../api'
import { useForm } from '../../composables/useForm'
import { errorMessage } from '../../errorMessage'
import { type CustomerDraft, customerChecks, toCustomerInput } from './customerForm'

const pageSize = 50
const fixTheFields = 'Confira os campos destacados.'
const conflictMessage = 'Outra pessoa alterou este cliente. Recarregue para ver a versão atual.'
const phoneTakenMessage = 'Já existe um cliente com este WhatsApp.'
/** orderPages caps how many pages of a customer's orders the drawer reads; a customer has few orders. */
const orderPages = 5
const orderPageSize = 100

/**
 * orderSequence reads the running number out of an order number such as
 * "P-0231". Numbers are handed out in the order orders are created, so
 * comparing them sorts by creation; it is an ordering, never arithmetic.
 */
function orderSequence(order: OrderSummary): number {
  return Number(order.number.replace(/\D/g, ''))
}

/**
 * searchTerm turns what was typed in "Buscar por nome ou WhatsApp" into what
 * the API takes. A number written the way people write it, "(92) 99123-4567",
 * is sent as its digits, because the API matches digits of the WhatsApp.
 */
export function searchTerm(typed: string): string | undefined {
  const text = typed.trim()
  if (text === '') return undefined
  if (/^[\d\s()+.-]+$/.test(text)) return text.replace(/\D/g, '')
  return text
}

/**
 * useCustomersStore backs Clientes: the customer list with its search, the
 * drawer with one customer (form, addresses and orders), and the saves. Every
 * role with `order:read` reads; the drawer is a form only for `customer:write`
 * (the view decides what to show, the API refuses the rest).
 */
export const useCustomersStore = defineStore('customers', () => {
  const items = shallowRef<CustomerSummary[]>([])
  const nextCursor = ref<string | undefined>(undefined)
  const loading = ref(false)
  /** loaded is true once a list read succeeded, so the screen tells "loading" from "empty". */
  const loaded = ref(false)
  const loadFailed = ref(false)
  /** query is the search box text; the list is read again with it by `search`. */
  const query = ref('')
  const notice = ref<string | null>(null)
  const error = ref<string | null>(null)
  /** timeZone is the store's, for the day of "Último pedido"; undefined until read, then the viewer's zone stands in. */
  const timeZone = ref<string | undefined>(undefined)
  let listRead = 0

  /** editing is the customer in the drawer, or null for a new one; `drawerOpen` says whether the drawer is there at all. */
  const editing = shallowRef<Customer | null>(null)
  const drawerOpen = ref(false)
  const drawerLoading = ref(false)
  /** drawerKey changes whenever the drawer's customer is read again, so the form starts over from it. */
  const drawerKey = ref(0)
  const conflict = ref(false)
  const form = useForm()

  const orders = shallowRef<OrderSummary[]>([])
  const ordersLoading = ref(false)
  const ordersError = ref<string | null>(null)
  /** ordersCapped is true when the customer has more orders than the drawer reads. */
  const ordersCapped = ref(false)
  let orderRead = 0

  async function readList(cursor: string | undefined): Promise<void> {
    const read = ++listRead
    loading.value = true
    try {
      const page = await listCustomers(api, { cursor, limit: pageSize, q: searchTerm(query.value) })
      // A newer search started meanwhile: its answer is the one to show.
      if (read !== listRead) return
      items.value = cursor === undefined ? page.items : [...items.value, ...page.items]
      nextCursor.value = page.nextCursor
      loaded.value = true
    } finally {
      if (read === listRead) loading.value = false
    }
  }

  /** guard runs a read and turns a refusal into the alert above the list. Programming errors are not caught. */
  async function guard<T>(call: () => Promise<T>): Promise<T | undefined> {
    error.value = null
    try {
      return await call()
    } catch (failure) {
      if (!(failure instanceof ApiError)) throw failure
      error.value = errorMessage(failure, { not_found: 'Cliente não encontrado.' })
      return undefined
    }
  }

  /**
   * readTimeZone reads the store's time zone so a date shows the day the
   * store lived it. Failing is harmless: dates then show in the viewer's zone.
   */
  async function readTimeZone(): Promise<void> {
    try {
      timeZone.value = (await getSettings(api)).timeZone
    } catch (failure) {
      if (!(failure instanceof ApiError)) throw failure
    }
  }

  /** open reads the first page of the list, and the store's time zone. */
  async function open(): Promise<void> {
    notice.value = null
    const [read] = await Promise.all([
      guard(async () => {
        await readList(undefined)
        return true
      }),
      readTimeZone(),
    ])
    loadFailed.value = read === undefined
  }

  /** search reads the first page again with the text in the search box. */
  async function search(): Promise<void> {
    await guard(() => readList(undefined))
  }

  /** loadMore appends the next page ("Carregar mais"). */
  async function loadMore(): Promise<void> {
    if (nextCursor.value !== undefined) await guard(() => readList(nextCursor.value))
  }

  /**
   * readOrders reads the customer's orders for "Pedidos deste cliente", newest
   * first. The API lists by delivery date, earliest first, and has no creation
   * date, so up to `orderPages` pages are read and sorted by order number,
   * which follows creation. When the cap is hit with pages left,
   * `ordersCapped` makes the drawer say it shows the nearest deliveries only.
   */
  async function readOrders(customerId: string): Promise<void> {
    const read = ++orderRead
    ordersLoading.value = true
    ordersError.value = null
    ordersCapped.value = false
    try {
      const all: OrderSummary[] = []
      let cursor: string | undefined
      for (let page = 0; page < orderPages; page += 1) {
        const answer = await listOrders(api, { customerId, scope: 'all', limit: orderPageSize, cursor })
        all.push(...answer.items)
        cursor = answer.nextCursor
        if (cursor === undefined) break
      }
      if (read !== orderRead) return
      orders.value = all.sort((a, b) => orderSequence(b) - orderSequence(a))
      ordersCapped.value = cursor !== undefined
    } catch (failure) {
      if (!(failure instanceof ApiError)) throw failure
      if (read === orderRead) ordersError.value = errorMessage(failure)
    } finally {
      if (read === orderRead) ordersLoading.value = false
    }
  }

  function showCustomer(customer: Customer | null): void {
    editing.value = customer
    orders.value = []
    ordersError.value = null
    ordersCapped.value = false
    drawerKey.value += 1
    form.clear()
    conflict.value = false
    drawerOpen.value = true
    if (customer !== null) void readOrders(customer.id)
  }

  /** openNew opens the drawer for a new customer. */
  function openNew(): void {
    notice.value = null
    showCustomer(null)
  }

  /** openCustomer reads the customer and opens the drawer on it; a customer that is not there leaves the message above the list. */
  async function openCustomer(customerId: string): Promise<void> {
    notice.value = null
    drawerLoading.value = true
    try {
      const customer = await guard(() => getCustomer(api, customerId))
      if (customer !== undefined) showCustomer(customer)
    } finally {
      drawerLoading.value = false
    }
  }

  /** closeDrawer closes it, dropping unsaved edits. */
  function closeDrawer(): void {
    orderRead += 1
    drawerOpen.value = false
    editing.value = null
    orders.value = []
    ordersLoading.value = false
    ordersCapped.value = false
    form.clear()
    conflict.value = false
  }

  /** reloadCustomer reads the open customer again ("Recarregar"), dropping the edits that lost the race. */
  async function reloadCustomer(): Promise<void> {
    const current = editing.value
    if (current === null) return
    try {
      showCustomer(await getCustomer(api, current.id))
    } catch (failure) {
      if (!(failure instanceof ApiError)) throw failure
      form.error.value = errorMessage(failure)
    }
  }

  /** save creates or replaces the customer from the form, and resolves whether it was saved. */
  async function save(draft: CustomerDraft): Promise<boolean> {
    conflict.value = false
    const current = editing.value
    const saved = await form.attempt(
      customerChecks(draft),
      {
        byCode: { version_conflict: conflictMessage, phone_taken: fixTheFields, invalid: fixTheFields },
        fieldByCode: { phone_taken: ['phone', phoneTakenMessage] },
      },
      async () => {
        const body = toCustomerInput(draft)
        try {
          if (current === null) await createCustomer(api, body)
          else await updateCustomer(api, current.id, { ...body, version: current.version })
        } catch (failure) {
          conflict.value = failure instanceof ApiError && failure.code === 'version_conflict'
          throw failure
        }
      },
    )
    if (!saved) return false
    notice.value = 'Cliente salvo.'
    closeDrawer()
    await search()
    return true
  }

  return {
    items,
    hasMore: computed(() => nextCursor.value !== undefined),
    loading,
    loaded,
    loadFailed,
    query,
    notice,
    error,
    timeZone,
    editing,
    drawerOpen,
    drawerLoading,
    drawerKey,
    conflict,
    saving: form.submitting,
    formError: form.error,
    fieldErrors: form.fieldErrors,
    orders,
    ordersLoading,
    ordersError,
    ordersCapped,
    open,
    search,
    loadMore,
    openNew,
    openCustomer,
    closeDrawer,
    reloadCustomer,
    save,
  }
})
