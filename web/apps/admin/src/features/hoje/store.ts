import { ApiError, getToday, type Today } from '@encomendo/api'
import { defineStore } from 'pinia'
import { ref, shallowRef } from 'vue'

import { api } from '../../api'
import { errorMessage } from '../../errorMessage'

/**
 * useHojeStore backs Hoje: the answer of GET /today as the server built it
 * (counts, "Precisa de você" and "Próximas entregas", for the store's day).
 * Nothing is counted or sorted here, so the screen can never disagree with
 * the server or with the Android shell.
 */
export const useHojeStore = defineStore('hoje', () => {
  const today = shallowRef<Today | null>(null)
  const loading = ref(false)
  /** loadError is why the last read failed; the screen then offers "Tentar de novo". */
  const loadError = ref<string | null>(null)
  let read = 0

  /** open reads Hoje again. A slower, older read never replaces a newer one. */
  async function open(): Promise<void> {
    const mine = ++read
    loading.value = true
    loadError.value = null
    try {
      const answer = await getToday(api)
      if (mine === read) today.value = answer
    } catch (failure) {
      if (!(failure instanceof ApiError)) throw failure
      if (mine === read) loadError.value = errorMessage(failure)
    } finally {
      if (mine === read) loading.value = false
    }
  }

  return { today, loading, loadError, open }
})
