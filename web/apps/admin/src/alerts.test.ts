import { describe, expect, it } from 'vitest'

import { worstAlert } from './alerts'

describe('worstAlert', () => {
  it('is null without an alert', () => {
    expect(worstAlert([])).toBeNull()
  })

  it('is "Pode atrasar" when every alert is at risk', () => {
    expect(worstAlert([{ kind: 'at_risk', itemId: 'a' }, { kind: 'at_risk', itemId: 'b' }])).toBe('at_risk')
  })

  it('is "Atrasada" as soon as one item is late, whatever the order', () => {
    expect(worstAlert([{ kind: 'at_risk', itemId: 'a' }, { kind: 'late', itemId: 'b' }])).toBe('late')
    expect(worstAlert([{ kind: 'late', itemId: 'b' }, { kind: 'at_risk', itemId: 'a' }])).toBe('late')
  })
})
