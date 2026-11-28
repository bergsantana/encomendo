/**
 * formatDate shows an instant from the API as a pt-BR date, such as
 * "02/12/2026", in the viewer's time zone unless one is given. Dates are
 * only displayed here; the backend calculates them.
 */
export function formatDate(iso: string, timeZone?: string): string {
  return new Intl.DateTimeFormat('pt-BR', { day: '2-digit', month: '2-digit', year: 'numeric', timeZone }).format(new Date(iso))
}

/**
 * formatCivilDate shows a calendar day from the API ("2026-12-12", such as a
 * delivery date) as "12/12/2026". A civil day has no time zone, so it is
 * written as it came and never moved by the viewer's zone.
 */
export function formatCivilDate(day: string): string {
  return formatDate(`${day}T12:00:00Z`, 'UTC')
}

/**
 * formatLongDay shows a calendar day from the API as the title of Hoje does,
 * "sexta-feira, 27 de novembro". Like `formatCivilDate` it never moves the day
 * by the viewer's zone.
 */
export function formatLongDay(day: string): string {
  return new Intl.DateTimeFormat('pt-BR', { weekday: 'long', day: 'numeric', month: 'long', timeZone: 'UTC' }).format(new Date(`${day}T12:00:00Z`))
}

/** formatMoney shows integer cents as currency, such as "R$ 129,90". The amount is never turned into a float for arithmetic. */
export function formatMoney(cents: number, currency: string): string {
  return new Intl.NumberFormat('pt-BR', { style: 'currency', currency }).format(cents / 100)
}

/** formatMargin shows a margin the server computed as "{valor} ({n}%)", from its cents and basis points. */
export function formatMargin(margin: { cents: number; basisPoints: number }, currency: string): string {
  const percent = new Intl.NumberFormat('pt-BR', { style: 'percent', maximumFractionDigits: 2 }).format(margin.basisPoints / 10_000)
  return `${formatMoney(margin.cents, currency)} (${percent})`
}

/** formatDays shows a number of days: "1 dia", "3 dias". */
export function formatDays(days: number): string {
  return days === 1 ? '1 dia' : `${days} dias`
}

// Whole part with optional thousands dots, or plain digits; a comma or a dot, then one or two decimals.
const moneyText = /^(\d{1,3}(?:\.\d{3})+|\d+)(?:[,.](\d{1,2}))?$/

/**
 * parseCents reads what a person typed in a money field ("129,90", "1.234,5",
 * "R$ 50") as integer cents, or null when it is not an amount. It works on
 * the digits, never on floats, so no cent is lost.
 */
export function parseCents(text: string): number | null {
  const match = moneyText.exec(text.replace(/^R\$\s*/, '').trim())
  if (match === null) return null
  const whole = (match[1] ?? '').replaceAll('.', '')
  if (whole.length > 12) return null
  return Number(whole) * 100 + Number((match[2] ?? '').padEnd(2, '0'))
}

/** centsToText fills a money field with integer cents, as "129,90". */
export function centsToText(cents: number): string {
  return `${Math.trunc(cents / 100)},${String(cents % 100).padStart(2, '0')}`
}

/**
 * storeDay is the calendar day it is now in the store's time zone, as
 * "YYYY-MM-DD". The client may read its own clock for this in two places
 * only: to place "Hoje" on the order's timeline, and to choose the bounds of
 * Financeiro's presets. Deadlines, alerts and every date of an order are the
 * API's, never worked out from this. A zone the browser does not know, or no
 * zone, falls back to the viewer's own.
 */
export function storeDay(timeZone?: string, now: Date = new Date()): string {
  const format = (zone: string | undefined) => new Intl.DateTimeFormat('en-CA', { timeZone: zone, year: 'numeric', month: '2-digit', day: '2-digit' }).format(now)
  try {
    return format(timeZone)
  } catch (failure) {
    if (!(failure instanceof RangeError)) throw failure
    return format(undefined)
  }
}
