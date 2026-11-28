/**
 * newIdempotencyKey makes the key for one user action, such as "Salvar" on a
 * new order or "Registrar pagamento". The store makes it when the action
 * starts and sends the same key again on every retry of that action, so the
 * server answers a retry with the first result instead of doing it twice. A
 * new action, even with the same fields, gets a new key (ADR 0030).
 */
export function newIdempotencyKey(): string {
  return crypto.randomUUID()
}

/** idempotencyHeaders is the request header that carries the key; the API requires it on creating an order and recording a payment. */
export function idempotencyHeaders(key: string): Record<string, string> {
  return { 'Idempotency-Key': key }
}
