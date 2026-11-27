// What the API takes: 55, a two-digit DDD and 8 or 9 digits (docs/api/openapi.yaml, SupplierPhone).
const apiPhone = /^55[1-9]{2}\d{8,9}$/

/**
 * supplierPhone turns what a person typed ("(11) 98765-4321") into the
 * digits the API takes ("5511987654321"): only digits, with the country code
 * 55 added when the number starts at the DDD. It does not judge the number;
 * `isSupplierPhone` does.
 */
export function supplierPhone(text: string): string {
  const digits = text.replace(/\D/g, '')
  // A number with country code has 12 or 13 digits; DDD 55 alone is 10 or 11.
  return digits.startsWith('55') && digits.length >= 12 ? digits : `55${digits}`
}

/** isSupplierPhone says whether a typed number, with DDD, has the shape the API accepts. */
export function isSupplierPhone(text: string): boolean {
  return text.replace(/\D/g, '') !== '' && apiPhone.test(supplierPhone(text))
}
