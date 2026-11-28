/**
 * Financeiro asks the API for a period, and the API wants its two ends as
 * days. The presets ("Este mês", "Mês passado", "Últimos 30 dias") are
 * choices of that filter, written as days from the store's today. This is the
 * one place the screen works with calendar days; money, balances and margins
 * are never worked out here, and the API refuses a period it does not accept
 * (from after to, or over 366 days).
 */

/** PeriodPreset is one way to choose the period; `custom` is "Escolher período". */
export type PeriodPreset = 'this_month' | 'last_month' | 'last_30_days' | 'custom'

/** Days is a range of calendar days, "YYYY-MM-DD", both ends included. */
export interface Days {
  from: string
  to: string
}

/** isPreset narrows a value typed in a choice to a preset. */
export function isPreset(value: string): value is PeriodPreset {
  return presetLabels.some((entry) => entry.value === value)
}

/** presetLabels are the words of the picker, in order. */
export const presetLabels: readonly { value: PeriodPreset; label: string }[] = [
  { value: 'this_month', label: 'Este mês' },
  { value: 'last_month', label: 'Mês passado' },
  { value: 'last_30_days', label: 'Últimos 30 dias' },
  { value: 'custom', label: 'Escolher período' },
]

const pad = (n: number): string => String(n).padStart(2, '0')

function dayText(date: Date): string {
  return `${date.getUTCFullYear()}-${pad(date.getUTCMonth() + 1)}-${pad(date.getUTCDate())}`
}

function parseDay(day: string): Date {
  return new Date(`${day}T00:00:00Z`)
}

/** daysFor returns the days of a preset, counted from `today`; `custom` has none of its own. */
export function daysFor(preset: Exclude<PeriodPreset, 'custom'>, today: string): Days {
  const day = parseDay(today)
  switch (preset) {
    case 'this_month':
      return { from: dayText(new Date(Date.UTC(day.getUTCFullYear(), day.getUTCMonth(), 1))), to: today }
    case 'last_month':
      return {
        from: dayText(new Date(Date.UTC(day.getUTCFullYear(), day.getUTCMonth() - 1, 1))),
        // Day 0 of this month is the last day of the one before.
        to: dayText(new Date(Date.UTC(day.getUTCFullYear(), day.getUTCMonth(), 0))),
      }
    case 'last_30_days':
      // Both ends count, so 30 days end today and start 29 days back.
      return { from: dayText(new Date(Date.UTC(day.getUTCFullYear(), day.getUTCMonth(), day.getUTCDate() - 29))), to: today }
  }
}
