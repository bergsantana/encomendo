import { describe, expect, it } from 'vitest'

import { centsToText, formatCivilDate, formatDate, formatDays, formatLongDay, formatMargin, formatMoney, parseCents, storeDay } from './format'

describe('formatDate', () => {
  it('shows a day the Brazilian way, in the given time zone', () => {
    expect(formatDate('2026-12-02T12:00:00Z', 'America/Sao_Paulo')).toBe('02/12/2026')
  })

  it('uses the local day, not the UTC one', () => {
    // 01:00 UTC is still the evening before in São Paulo.
    expect(formatDate('2026-12-02T01:00:00Z', 'America/Sao_Paulo')).toBe('01/12/2026')
  })
})

describe('formatCivilDate', () => {
  it("shows the day as written, whatever the viewer's time zone", () => {
    expect(formatCivilDate('2026-12-12')).toBe('12/12/2026')
    expect(formatCivilDate('2026-01-01')).toBe('01/01/2026')
  })
})

describe('formatLongDay', () => {
  it("shows the weekday, the day and the month, whatever the viewer's zone", () => {
    expect(formatLongDay('2026-11-27')).toBe('sexta-feira, 27 de novembro')
    expect(formatLongDay('2026-12-01')).toBe('terça-feira, 1 de dezembro')
  })
})

describe('money', () => {
  it('shows cents as pt-BR currency', () => {
    expect(formatMoney(12990, 'BRL').replace(/\s/g, ' ')).toBe('R$ 129,90')
  })

  it('shows a margin as value and percent, from basis points', () => {
    expect(formatMargin({ cents: 4500, basisPoints: 3000 }, 'BRL').replace(/\s/g, ' ')).toBe('R$ 45,00 (30%)')
    expect(formatMargin({ cents: 1000, basisPoints: 3333 }, 'BRL').replace(/\s/g, ' ')).toBe('R$ 10,00 (33,33%)')
  })

  it('says "1 dia" and "n dias"', () => {
    expect(formatDays(1)).toBe('1 dia')
    expect(formatDays(0)).toBe('0 dias')
    expect(formatDays(15)).toBe('15 dias')
  })

  it.each([
    ['129,90', 12990],
    ['129,9', 12990],
    ['129', 12900],
    ['1.234,56', 123456],
    ['R$ 50', 5000],
    ['12.5', 1250],
    ['0,05', 5],
  ])('reads %s as %i cents', (text, cents) => {
    expect(parseCents(text)).toBe(cents)
  })

  it.each(['', 'abc', '12,345', '-5', '1,2,3', '9999999999999'])('refuses %s', (text) => {
    expect(parseCents(text)).toBeNull()
  })

  it('writes cents back into a field', () => {
    expect(centsToText(12990)).toBe('129,90')
    expect(centsToText(5)).toBe('0,05')
    expect(parseCents(centsToText(123456))).toBe(123456)
  })
})

describe('storeDay', () => {
  it('gives the day it is in the store\'s zone, not in UTC', () => {
    const instant = new Date('2026-12-01T03:30:00Z')
    expect(storeDay('America/Manaus', instant)).toBe('2026-11-30')
    expect(storeDay('America/Sao_Paulo', instant)).toBe('2026-12-01')
  })

  it('falls back to the viewer\'s zone for no zone or a zone the browser does not know', () => {
    const instant = new Date('2026-11-27T15:00:00Z')
    expect(storeDay(undefined, instant)).toMatch(/^\d{4}-\d{2}-\d{2}$/)
    expect(() => storeDay('Not/AZone', instant)).not.toThrow()
    expect(storeDay('Not/AZone', instant)).toBe(storeDay(undefined, instant))
  })
})
