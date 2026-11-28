import type { PaymentRequest } from '@encomendo/api'

/** PaymentKind is the deposit ("Sinal") or the balance ("Saldo"). */
export type PaymentKind = PaymentRequest['kind']
/** PaymentMethod is how a payment was received. */
export type PaymentMethod = PaymentRequest['method']

/** paymentMethods are the choices of "Forma", in the design document's order. */
export const paymentMethods: readonly { value: PaymentMethod; label: string }[] = [
  { value: 'pix', label: 'Pix' },
  { value: 'dinheiro', label: 'Dinheiro' },
  { value: 'cartao_credito', label: 'Cartão de crédito' },
  { value: 'cartao_debito', label: 'Cartão de débito' },
  { value: 'transferencia', label: 'Transferência' },
  { value: 'outro', label: 'Outro' },
]

/** paymentKindLabel is "Sinal" or "Saldo"; a kind the screen does not know yet is shown as the server wrote it. */
export function paymentKindLabel(kind: string): string {
  if (kind === 'deposit') return 'Sinal'
  return kind === 'balance' ? 'Saldo' : kind
}

/** paymentMethodLabel is the word of a payment method; a method the screen does not know yet is shown as the server wrote it. */
export function paymentMethodLabel(method: string): string {
  return paymentMethods.find((choice) => choice.value === method)?.label ?? method
}
