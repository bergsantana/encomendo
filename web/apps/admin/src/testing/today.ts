import type { FinanceSummary, Today } from '@encomendo/api'
import { http, HttpResponse } from 'msw'

import { brl } from './orders'

/**
 * Fixtures for Hoje and Financeiro: the answers of GET /today and
 * GET /finance/summary. Every number is written out here, as the API would
 * send it, so a test can tell a screen that shows the answer from one that
 * works it out.
 */

/** emptyToday is a store with nothing active. */
export const emptyToday: Today = {
  today: '2026-11-27',
  counts: { active: 0, awaitingDeposit: 0, withAlert: 0, dueIn7Days: 0 },
  needsYou: [],
  upcoming: [],
  truncated: false,
}

/** todayHandler answers GET /today with the given view; with none, the empty one. Tests that only need Hoje to load use it. */
export function todayHandler(view: Today = emptyToday) {
  return http.get('*/api/v1/admin/today', () => HttpResponse.json(view))
}

/** emptyFinance is a period with no payment, no balance and no handed-over order. */
export function emptyFinance(from = '2026-11-01', to = '2026-11-27'): FinanceSummary {
  return {
    from,
    to,
    received: { total: brl(0), byKind: [], byMethod: [] },
    toReceive: { total: brl(0), orders: 0 },
    toRefund: [],
    margins: [],
    marginsTotal: { total: brl(0), cost: brl(0), margin: null },
    truncated: false,
  }
}
