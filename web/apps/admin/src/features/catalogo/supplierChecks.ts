import type { FieldCheck } from '../../composables/useForm'
import { isWhatsappNumber } from '../../phone'

/**
 * supplierChecks are the checks of a supplier's name and WhatsApp before the
 * API is called, with the wording both "Novo fornecedor" in Catálogo and the
 * Fornecedores tab of Compras use. The number is typed with its DDD.
 */
export function supplierChecks(name: string, whatsapp: string): FieldCheck[] {
  return [
    ['name', name.trim(), 'Informe o nome do fornecedor.'],
    ['whatsappPhone', whatsapp.trim(), 'Informe o WhatsApp do fornecedor.'],
    ['whatsappPhone', isWhatsappNumber(whatsapp) || whatsapp.trim() === '' ? 'ok' : '', 'Confira o número: use DDD e 8 ou 9 dígitos.'],
  ]
}
