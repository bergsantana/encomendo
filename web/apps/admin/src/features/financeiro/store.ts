import { ApiError, type FinanceSummary, getFinanceSummary, getSettings } from '@encomendo/api'
import { defineStore } from 'pinia'
import { ref, shallowRef } from 'vue'

import { api } from '../../api'
import { errorMessage } from '../../errorMessage'
import { storeDay } from '../../format'
import { daysFor, type PeriodPreset } from './period'

const badPeriod = 'Confira o período: o início não pode passar do fim, e o período tem no máximo 366 dias.'

/**
 * useFinanceStore backs Financeiro, the owner's money view. It keeps the
 * period the owner chose, sends its two ends to the API and shows the summary
 * that comes back: received by kind and method, "A receber", "A devolver" and
 * the margins, with the period's margin total. It adds up nothing itself.
 * Only the owner (`finance:read`) opens the screen; the API checks it again.
 */
export const useFinanceStore = defineStore('finance', () => {
  const summary = shallowRef<FinanceSummary | null>(null)
  const preset = ref<PeriodPreset>('this_month')
  /** from and to are the days the owner chose; they are what the last read sent. */
  const from = ref('')
  const to = ref('')
  const loading = ref(false)
  const error = ref<string | null>(null)
  /** today is the store's day, from its time zone; known once `open` has run. */
  let today = ''
  let read = 0

  // The figures of another period never stay on screen under the new one: they go while it loads or fails.
  async function load(): Promise<void> {
    const mine = ++read
    loading.value = true
    error.value = null
    summary.value = null
    try {
      const answer = await getFinanceSummary(api, { from: from.value, to: to.value })
      if (mine === read) summary.value = answer
    } catch (failure) {
      if (!(failure instanceof ApiError)) throw failure
      if (mine === read) error.value = errorMessage(failure, { invalid: badPeriod })
    } finally {
      if (mine === read) loading.value = false
    }
  }

  /**
   * open starts on "Este mês" for the store's day. The store's time zone comes
   * from the settings; when they cannot be read, the viewer's zone stands in.
   */
  async function open(): Promise<void> {
    loading.value = true
    error.value = null
    summary.value = null
    let timeZone: string | undefined
    try {
      timeZone = (await getSettings(api)).timeZone
    } catch (failure) {
      if (!(failure instanceof ApiError)) throw failure
    }
    today = storeDay(timeZone)
    await choose('this_month')
  }

  /**
   * choose changes the period to a preset and reads it. "Escolher período"
   * reads nothing yet: the fields start on the days already shown, and the
   * owner's change of either one reads (`setDays`).
   */
  async function choose(next: PeriodPreset): Promise<void> {
    preset.value = next
    if (next === 'custom') return
    const days = daysFor(next, today)
    from.value = days.from
    to.value = days.to
    await load()
  }

  /** setDays sets the days of "Escolher período" and reads once both are filled in. */
  async function setDays(start: string, end: string): Promise<void> {
    from.value = start
    to.value = end
    if (start !== '' && end !== '') await load()
  }

  /** retry reads the period again after a failure, keeping the one chosen; before the first read it starts over. */
  async function retry(): Promise<void> {
    if (today === '') await open()
    else if (from.value !== '' && to.value !== '') await load()
  }

  return { summary, preset, from, to, loading, error, open, choose, setDays, retry }
})
