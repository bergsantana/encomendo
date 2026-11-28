import { ApiError, getToday, type PurchasingToday, purchasingToday, type Today } from '@encomendo/api'
import { defineStore } from 'pinia'
import { ref, shallowRef } from 'vue'

import { api } from '../../api'
import { errorMessage } from '../../errorMessage'
import { useSessionStore } from '../../stores/session'

/**
 * useHojeStore backs Hoje: the answer of GET /today as the server built it
 * (counts, "Precisa de você" and "Próximas entregas", for the store's day).
 * Nothing is counted or sorted here, so the screen can never disagree with
 * the server or with the Android shell. The roles that buy (`purchase:write`)
 * also read what purchasing needs today ("Compras a enviar" and its entries
 * of "Precisa de você"); the other roles never call that endpoint.
 */
export const useHojeStore = defineStore('hoje', () => {
  const session = useSessionStore()
  const today = shallowRef<Today | null>(null)
  /** purchasing is Compras' part of Hoje, or null for a role that does not buy or when it could not be read. */
  const purchasing = shallowRef<PurchasingToday | null>(null)
  const loading = ref(false)
  /** loadError is why the last read failed; the screen then offers "Tentar de novo". */
  const loadError = ref<string | null>(null)
  let read = 0

  /** readPurchasing reads Compras' part of Hoje; Hoje works without it, so a failed read leaves it out and says nothing. */
  async function readPurchasing(): Promise<PurchasingToday | null> {
    // The endpoint is purchase:write only: skipping the call spares every other role a 403 on each load of Hoje. The API still decides.
    if (!session.can('purchase:write')) return null
    try {
      return await purchasingToday(api)
    } catch (failure) {
      if (!(failure instanceof ApiError)) throw failure
      return null
    }
  }

  /** open reads Hoje again. A slower, older read never replaces a newer one. */
  async function open(): Promise<void> {
    const mine = ++read
    loading.value = true
    loadError.value = null
    try {
      const [answer, purchases] = await Promise.all([getToday(api), readPurchasing()])
      if (mine === read) {
        today.value = answer
        purchasing.value = purchases
      }
    } catch (failure) {
      if (!(failure instanceof ApiError)) throw failure
      if (mine === read) loadError.value = errorMessage(failure)
    } finally {
      if (mine === read) loading.value = false
    }
  }

  return { today, purchasing, loading, loadError, open }
})
