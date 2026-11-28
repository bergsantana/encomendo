// What the API takes for a WhatsApp number: 55, a two-digit DDD and 8 or 9 digits
// (docs/api/openapi.yaml: SupplierPhone, CustomerPhone, WhatsappPhone).
const apiPhone = /^55[1-9]{2}\d{8,9}$/

/**
 * whatsappDigits turns what a person typed ("(11) 98765-4321") into the
 * digits the API takes ("5511987654321"): only digits, with the country code
 * 55 added when the number starts at the DDD. It does not judge the number;
 * `isWhatsappNumber` does. Suppliers, customers and the store's own number
 * all use it.
 */
export function whatsappDigits(text: string): string {
  const digits = text.replace(/\D/g, '')
  // A number with country code has 12 or 13 digits; DDD 55 alone is 10 or 11.
  return digits.startsWith('55') && digits.length >= 12 ? digits : `55${digits}`
}

/** isWhatsappNumber says whether a typed number, with DDD, has the shape the API accepts. */
export function isWhatsappNumber(text: string): boolean {
  return text.replace(/\D/g, '') !== '' && apiPhone.test(whatsappDigits(text))
}

/**
 * formatWhatsapp shows the API's digits the way people write them,
 * "(92) 99123-4567", so a number read from the API can fill a field and be
 * typed over. Anything that is not the API's shape is returned as it came.
 */
export function formatWhatsapp(digits: string): string {
  if (!apiPhone.test(digits)) return digits
  const local = digits.slice(4)
  const split = local.length - 4
  return `(${digits.slice(2, 4)}) ${local.slice(0, split)}-${local.slice(split)}`
}
