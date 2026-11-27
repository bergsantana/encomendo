import { describe, expect, it } from 'vitest'

import { isSupplierPhone, supplierPhone } from './phone'

describe('supplierPhone', () => {
  it.each([
    ['(11) 98765-4321', '5511987654321'],
    ['11 3456-7890', '551134567890'],
    ['+55 (11) 98765-4321', '5511987654321'],
    ['5511987654321', '5511987654321'],
  ])('writes %s as %s', (typed, sent) => {
    expect(supplierPhone(typed)).toBe(sent)
  })

  it('does not mistake DDD 55 for the country code', () => {
    // Rio Grande do Sul: DDD 55, then 9 or 8 digits (11 or 10 digits in all).
    expect(supplierPhone('(55) 99876-5432')).toBe('5555998765432')
    expect(supplierPhone('55 3456-7890')).toBe('555534567890')
  })

  it('passes a leading zero on, and refuses it', () => {
    expect(supplierPhone('011 98765-4321')).toBe('55011987654321')
    expect(isSupplierPhone('011 98765-4321')).toBe(false)
  })

  it('writes an empty field as just the country code, which is never valid', () => {
    expect(supplierPhone('')).toBe('55')
    expect(isSupplierPhone('')).toBe(false)
  })
})

describe('isSupplierPhone', () => {
  it.each(['(11) 98765-4321', '11 3456-7890', '+55 11 98765-4321', '(55) 99876-5432', '55 3456-7890'])('accepts %s', (typed) => {
    expect(isSupplierPhone(typed)).toBe(true)
  })

  it.each(['1234', '(11) 8765-43', '(01) 98765-4321', '(11) 98765-43210', 'abc'])('refuses %s', (typed) => {
    expect(isSupplierPhone(typed)).toBe(false)
  })
})
