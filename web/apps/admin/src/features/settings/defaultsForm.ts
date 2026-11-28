import type { TenantSettings, TimeZone, UpdateSettingsRequest } from '@encomendo/api'

import type { FieldCheck } from '../../composables/useForm'
import { formatWhatsapp, isWhatsappNumber, whatsappDigits } from '../../phone'

/**
 * DefaultsDraft is the "Prazos e sinal", "Compras", "Estoque e Produção",
 * "WhatsApp da loja" and "Fuso horário" fields as the person edits them:
 * numbers as text, the WhatsApp number as typed.
 */
export interface DefaultsDraft {
  depositPercent: string
  assemblyDays: string
  safetyMarginDays: string
  purchaseBeforeDeposit: boolean
  stockEnabled: boolean
  productionEnabled: boolean
  whatsapp: string
  timeZone: TimeZone
}

/** timeZoneChoices are the four zones, named as the design document names them (docs/product, Configurações). */
export const timeZoneChoices: { value: TimeZone; label: string }[] = [
  { value: 'America/Sao_Paulo', label: 'Horário de Brasília' },
  { value: 'America/Manaus', label: 'Horário do Amazonas (Manaus)' },
  { value: 'America/Rio_Branco', label: 'Horário do Acre (Rio Branco)' },
  { value: 'America/Noronha', label: 'Horário de Fernando de Noronha' },
]

/** draftFromSettings fills the form from the settings the API sent. */
export function draftFromSettings(settings: TenantSettings): DefaultsDraft {
  return {
    depositPercent: String(settings.depositPercent),
    assemblyDays: String(settings.assemblyDays),
    safetyMarginDays: String(settings.safetyMarginDays),
    purchaseBeforeDeposit: settings.purchaseBeforeDeposit,
    stockEnabled: settings.stockEnabled,
    productionEnabled: settings.productionEnabled,
    whatsapp: settings.whatsappPhone === null ? '' : formatWhatsapp(settings.whatsappPhone),
    // A store always has one; the contract only leaves it optional on an update.
    timeZone: settings.timeZone ?? 'America/Sao_Paulo',
  }
}

/** whole reads digits only, so "3,5" or "-1" is not a number of days. */
function whole(text: string): number | null {
  return /^\d{1,4}$/.test(text.trim()) ? Number(text.trim()) : null
}

function within(max: number): (text: string) => boolean {
  return (text) => {
    const value = whole(text)
    return value !== null && value <= max
  }
}

/** defaultsChecks are the checks run before the call; the API checks the same limits again. */
export function defaultsChecks(draft: DefaultsDraft): FieldCheck[] {
  const days = 'Use de 0 a 365 dias.'
  return [
    ['depositPercent', draft.depositPercent, 'Use um sinal de 0 a 100%.', within(100)],
    ['assemblyDays', draft.assemblyDays, days, within(365)],
    ['safetyMarginDays', draft.safetyMarginDays, days, within(365)],
    ['whatsappPhone', draft.whatsapp, 'Confira o número: use DDD e 8 ou 9 dígitos.', (text) => text.trim() === '' || isWhatsappNumber(text)],
  ]
}

/**
 * toUpdateRequest builds the body of the save: every field, with the
 * `version` read and the time zone. An empty WhatsApp field clears the number.
 */
export function toUpdateRequest(draft: DefaultsDraft, version: number): UpdateSettingsRequest {
  return {
    stockEnabled: draft.stockEnabled,
    productionEnabled: draft.productionEnabled,
    purchaseBeforeDeposit: draft.purchaseBeforeDeposit,
    depositPercent: Number(draft.depositPercent.trim()),
    assemblyDays: Number(draft.assemblyDays.trim()),
    safetyMarginDays: Number(draft.safetyMarginDays.trim()),
    whatsappPhone: draft.whatsapp.trim() === '' ? null : whatsappDigits(draft.whatsapp),
    timeZone: draft.timeZone,
    version,
  }
}
