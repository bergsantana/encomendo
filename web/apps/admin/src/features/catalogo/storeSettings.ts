import { getSettings } from '@encomendo/api'
import { ref } from 'vue'

import { api } from '../../api'

/**
 * useStoreSettings reads the store settings Catálogo depends on: which
 * sourcing modes are on, the currency, and the default assembly and safety
 * margin days a kit shows as "Padrão da loja". The catalog store uses it for
 * both tabs, so there is one copy.
 */
export function useStoreSettings() {
  const stockEnabled = ref(false)
  const productionEnabled = ref(false)
  const currency = ref('BRL')
  /** assemblyDays and safetyMarginDays are null until the settings are read. */
  const assemblyDays = ref<number | null>(null)
  const safetyMarginDays = ref<number | null>(null)

  async function load(): Promise<void> {
    const settings = await getSettings(api)
    stockEnabled.value = settings.stockEnabled
    productionEnabled.value = settings.productionEnabled
    currency.value = settings.currency
    assemblyDays.value = settings.assemblyDays
    safetyMarginDays.value = settings.safetyMarginDays
  }

  return { stockEnabled, productionEnabled, currency, assemblyDays, safetyMarginDays, load }
}
