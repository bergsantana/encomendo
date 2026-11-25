import { describe, expect, it } from 'vitest'

import { formatDate } from './format'

describe('formatDate', () => {
  it('shows a day the Brazilian way, in the given time zone', () => {
    expect(formatDate('2026-12-02T12:00:00Z', 'America/Sao_Paulo')).toBe('02/12/2026')
  })

  it('uses the local day, not the UTC one', () => {
    // 01:00 UTC is still the evening before in São Paulo.
    expect(formatDate('2026-12-02T01:00:00Z', 'America/Sao_Paulo')).toBe('01/12/2026')
  })
})
