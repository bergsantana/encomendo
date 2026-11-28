function dayNumber(day: string): number {
  const [year = 0, month = 1, date = 1] = day.split('-').map(Number)
  return Date.UTC(year, month - 1, date) / 86_400_000
}

/**
 * bandPosition places a day on the timeline's band, as a percentage from its
 * first day to its last (clamped to 0–100). It is layout geometry for the
 * drawing only: no date of the order is computed from it, and the phone
 * layout, which has no band, shows the same dates as text.
 */
export function bandPosition(day: string, from: string, to: string): number {
  const span = dayNumber(to) - dayNumber(from)
  if (span <= 0) return 100
  const position = ((dayNumber(day) - dayNumber(from)) / span) * 100
  return Math.min(100, Math.max(0, position))
}

/** latestDay is the last of some "YYYY-MM-DD" days (they sort as text), or the fallback when there are none. */
export function latestDay(days: readonly string[], fallback: string): string {
  return days.reduce((latest, day) => (day > latest ? day : latest), fallback)
}
