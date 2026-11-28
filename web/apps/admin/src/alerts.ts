import type { Alert } from '@encomendo/api'

/**
 * worstAlert is the alert an order shows on its status: "Atrasada" over
 * "Pode atrasar", or null when it has none. Every list that draws an order's
 * alert (Pedidos, Hoje) uses it, so they say the same thing.
 */
export function worstAlert(alerts: readonly Alert[]): Alert['kind'] | null {
  if (alerts.some((alert) => alert.kind === 'late')) return 'late'
  return alerts.length > 0 ? 'at_risk' : null
}
