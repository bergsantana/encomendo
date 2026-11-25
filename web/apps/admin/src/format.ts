/**
 * formatDate shows an instant from the API as a pt-BR date, such as
 * "02/12/2026", in the viewer's time zone unless one is given. Dates are
 * only displayed here; the backend calculates them.
 */
export function formatDate(iso: string, timeZone?: string): string {
  return new Intl.DateTimeFormat('pt-BR', { day: '2-digit', month: '2-digit', year: 'numeric', timeZone }).format(new Date(iso))
}
