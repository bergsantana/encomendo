import { ApiError, getSettings, listOrders, type OrderListFilter, type OrderPhase, type OrderStatus, type OrderSummary } from '@encomendo/api'
import { defineStore } from 'pinia'
import { computed, ref, shallowRef } from 'vue'

import { api } from '../../api'
import { errorMessage } from '../../errorMessage'

const pageSize = 50

/** activeTab, handedOverTab and cancelledTab are the fixed tabs; a status tab is `status:{key}`. */
export const activeTab = 'active'
export const handedOverTab = 'handed_over'
export const cancelledTab = 'cancelled'

/** OrderTab is one tab of the order list. */
export interface OrderTab {
  id: string
  label: string
}

/** statusTabId is the tab of one status of the store's current list. */
export function statusTabId(key: string): string {
  return `status:${key}`
}

/**
 * filterForTab is what the API is asked for by a tab: "Ativos" are the
 * default scope, a status tab is one status among the active orders,
 * "Entregues" and "Cancelados" are their scopes.
 */
function filterForTab(tab: string): OrderListFilter {
  if (tab === handedOverTab) return { scope: 'handed_over' }
  if (tab === cancelledTab) return { scope: 'cancelled' }
  if (tab.startsWith('status:')) return { status: tab.slice('status:'.length) }
  return {}
}

/**
 * usePedidosStore backs the order list: the tabs (one per status of the
 * store's current list, so their words come from the workflow), the search,
 * "Só com alerta" and paging. Alerts and dates come from the API as they are.
 */
export const usePedidosStore = defineStore('pedidos', () => {
  const items = shallowRef<OrderSummary[]>([])
  const statuses = shallowRef<OrderStatus[]>([])
  const nextCursor = ref<string | undefined>(undefined)
  const loading = ref(false)
  /** loaded is true once a list read succeeded, so the screen tells "loading" from "empty". */
  const loaded = ref(false)
  const loadFailed = ref(false)
  const error = ref<string | null>(null)
  const tab = ref(activeTab)
  const query = ref('')
  const alertOnly = ref(false)
  /** phase narrows the list to one phase, as a link from Hoje does ("Aguardando sinal"); choosing a tab drops it. */
  const phase = ref<OrderPhase | undefined>(undefined)
  let listRead = 0

  /**
   * tabs are "Ativos", the statuses of the store's current list that are not
   * the end of an order, then "Entregues" and "Cancelados". A status that
   * hands the order over is the "Entregues" tab, so it is not listed twice.
   */
  const tabs = computed<OrderTab[]>(() => [
    { id: activeTab, label: 'Ativos' },
    ...statuses.value
      .filter((status) => status.phase !== 'handed_over' && status.phase !== 'cancelled')
      .map((status) => ({ id: statusTabId(status.key), label: status.label })),
    { id: handedOverTab, label: 'Entregues' },
    { id: cancelledTab, label: 'Cancelados' },
  ])

  async function readList(cursor: string | undefined): Promise<void> {
    const read = ++listRead
    loading.value = true
    error.value = null
    try {
      const text = query.value.trim()
      const page = await listOrders(api, {
        ...filterForTab(tab.value),
        phase: phase.value,
        cursor,
        limit: pageSize,
        q: text === '' ? undefined : text,
        alert: alertOnly.value,
      })
      // A newer read started meanwhile: its answer is the one to show.
      if (read !== listRead) return
      items.value = cursor === undefined ? page.items : [...items.value, ...page.items]
      nextCursor.value = page.nextCursor
      loaded.value = true
    } catch (failure) {
      if (!(failure instanceof ApiError)) throw failure
      if (read === listRead) error.value = errorMessage(failure)
      throw failure
    } finally {
      if (read === listRead) loading.value = false
    }
  }

  /** attempt runs a read and keeps its refusal as the message above the list; it resolves whether it worked. */
  async function attempt(read: () => Promise<void>): Promise<boolean> {
    try {
      await read()
      return true
    } catch (failure) {
      if (!(failure instanceof ApiError)) throw failure
      return false
    }
  }

  /** readStatuses reads the store's current status list for the tabs. A failure leaves only the fixed tabs. */
  async function readStatuses(): Promise<void> {
    try {
      statuses.value = (await getSettings(api)).statusWorkflow.statuses
    } catch (failure) {
      if (!(failure instanceof ApiError)) throw failure
    }
  }

  /** Filters are what a link into Pedidos can preset: a tab, a phase, and "Só com alerta". */
  interface Filters {
    tab?: string
    phase?: OrderPhase
    alertOnly?: boolean
  }

  /** open presets the filters, reads the status list and the first page. */
  async function open(filters: Filters = {}): Promise<void> {
    tab.value = filters.tab ?? activeTab
    alertOnly.value = filters.alertOnly ?? false
    phase.value = filters.phase
    query.value = ''
    loaded.value = false
    items.value = []
    const [, read] = await Promise.all([readStatuses(), attempt(() => readList(undefined))])
    loadFailed.value = !read
  }

  /** reload reads the first page again with the tab, search and alert filter as they stand. */
  async function reload(): Promise<void> {
    loadFailed.value = !(await attempt(() => readList(undefined)))
  }

  /** select opens a tab. */
  async function select(id: string): Promise<void> {
    tab.value = id
    phase.value = undefined
    await reload()
  }

  /** loadMore appends the next page ("Carregar mais"). */
  async function loadMore(): Promise<void> {
    if (nextCursor.value !== undefined) await attempt(() => readList(nextCursor.value))
  }

  return {
    items,
    tabs,
    hasMore: computed(() => nextCursor.value !== undefined),
    loading,
    loaded,
    loadFailed,
    error,
    tab,
    query,
    alertOnly,
    phase,
    open,
    reload,
    select,
    loadMore,
  }
})
