/**
 * newIdempotencyKey makes the key for one user action, such as "Salvar" on a
 * new order or "Registrar pagamento". The store makes it when the action
 * starts and sends the same key again on every retry of that action, so the
 * server answers a retry with the first result instead of doing it twice. A
 * new action, even with the same fields, gets a new key (ADR 0030).
 *
 * `crypto.randomUUID` exists only in secure contexts, so a plain-HTTP dev URL
 * (the Android emulator's http://10.0.2.2) has none; the key is then a
 * version 4 UUID built from `crypto.getRandomValues`, which works anywhere.
 */
export function newIdempotencyKey(): string {
  if (typeof crypto.randomUUID === 'function') return crypto.randomUUID()
  return randomUuidV4()
}

function randomUuidV4(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(16))
  // Version 4 in the high nibble of byte 6, and the RFC 4122 variant in byte 8.
  bytes[6] = ((bytes[6] ?? 0) & 0x0f) | 0x40
  bytes[8] = ((bytes[8] ?? 0) & 0x3f) | 0x80
  const hex = Array.from(bytes, (byte) => byte.toString(16).padStart(2, '0')).join('')
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`
}

/** idempotencyHeaders is the request header that carries the key; the API requires it on creating an order and recording a payment. */
export function idempotencyHeaders(key: string): Record<string, string> {
  return { 'Idempotency-Key': key }
}
