import { describe, expect, it } from 'vitest'

import { daysFor } from './period'

describe('daysFor', () => {
  it('starts "Este mês" on the first and ends on the store day', () => {
    expect(daysFor('this_month', '2026-11-27')).toEqual({ from: '2026-11-01', to: '2026-11-27' })
    expect(daysFor('this_month', '2026-12-01')).toEqual({ from: '2026-12-01', to: '2026-12-01' })
  })

  it('covers all of last month, whatever its length', () => {
    expect(daysFor('last_month', '2026-11-27')).toEqual({ from: '2026-10-01', to: '2026-10-31' })
    expect(daysFor('last_month', '2026-03-10')).toEqual({ from: '2026-02-01', to: '2026-02-28' })
    expect(daysFor('last_month', '2028-03-10')).toEqual({ from: '2028-02-01', to: '2028-02-29' })
    expect(daysFor('last_month', '2027-01-15')).toEqual({ from: '2026-12-01', to: '2026-12-31' })
  })

  it('counts 30 days ending today, both ends included', () => {
    expect(daysFor('last_30_days', '2026-11-27')).toEqual({ from: '2026-10-29', to: '2026-11-27' })
    expect(daysFor('last_30_days', '2027-01-10')).toEqual({ from: '2026-12-12', to: '2027-01-10' })
  })
})
