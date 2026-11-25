import { ApiError, getReadiness } from '@encomendo/api'
import { defineStore } from 'pinia'
import { ref } from 'vue'

import { api } from '../../api'

/** How the backend looked on the last check. */
export type ApiStatus = 'checking' | 'ok' | 'unavailable'

/**
 * useHojeStore backs the Hoje screen. For now it only checks that the API is
 * reachable; the summary cards and "Precisa de você" land in milestone 4.
 */
export const useHojeStore = defineStore('hoje', () => {
  const apiStatus = ref<ApiStatus>('checking')

  async function checkApi(): Promise<void> {
    apiStatus.value = 'checking'
    try {
      await getReadiness(api)
      apiStatus.value = 'ok'
    } catch (error) {
      if (!(error instanceof ApiError)) throw error
      apiStatus.value = 'unavailable'
    }
  }

  return { apiStatus, checkApi }
})
