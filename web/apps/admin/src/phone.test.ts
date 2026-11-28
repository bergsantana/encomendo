import { describe, expect, it } from 'vitest'

import { formatWhatsapp, isWhatsappNumber, whatsappDigits } from './phone'

describe('whatsappDigits', () => {
  it.each([
    ['(11) 98765-4321', '5511987654321'],
    ['11 3456-7890', '551134567890'],
    ['+55 (11) 98765-4321', '5511987654321'],
    ['5511987654321', '5511987654321'],
  ])('writes %s as %s', (typed, sent) => {
    expect(whatsappDigits(typed)).toBe(sent)
  })

  it('does not mistake DDD 55 for the country code', () => {
    // Rio Grande do Sul: DDD 55, then 9 or 8 digits (11 or 10 digits in all).
    expect(whatsappDigits('(55) 99876-5432')).toBe('5555998765432')
    expect(whatsappDigits('55 3456-7890')).toBe('555534567890')
  })

  it('passes a leading zero on, and refuses it', () => {
    expect(whatsappDigits('011 98765-4321')).toBe('55011987654321')
    expect(isWhatsappNumber('011 98765-4321')).toBe(false)
  })

  it('writes an empty field as just the country code, which is never valid', () => {
    expect(whatsappDigits('')).toBe('55')
    expect(isWhatsappNumber('')).toBe(false)
  })
})

describe('isWhatsappNumber', () => {
  it.each(['(11) 98765-4321', '11 3456-7890', '+55 11 98765-4321', '(55) 99876-5432', '55 3456-7890'])('accepts %s', (typed) => {
    expect(isWhatsappNumber(typed)).toBe(true)
  })

  it.each(['1234', '(11) 8765-43', '(01) 98765-4321', '(11) 98765-43210', 'abc'])('refuses %s', (typed) => {
    expect(isWhatsappNumber(typed)).toBe(false)
  })
})

describe('formatWhatsapp', () => {
  it.each([
    ['5592991234567', '(92) 99123-4567'],
    ['551134567890', '(11) 3456-7890'],
  ])('writes %s as %s', (digits, shown) => {
    expect(formatWhatsapp(digits)).toBe(shown)
  })

  it('returns what is not the API shape as it came', () => {
    expect(formatWhatsapp('')).toBe('')
    expect(formatWhatsapp('123')).toBe('123')
  })

  it('round-trips through whatsappDigits', () => {
    expect(whatsappDigits(formatWhatsapp('5592991234567'))).toBe('5592991234567')
  })
})
