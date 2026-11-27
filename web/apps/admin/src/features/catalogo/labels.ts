import type { SelectOption } from '@encomendo/ui'

import type { SourcingMode } from './itemForm'

// Product wording for how an item is sourced, the same in every store
// (docs/product/design-document.md, Catálogo). Sourcing modes are catalog
// data, not order statuses, so the code may branch on them.
const sourcing: Record<SourcingMode, string> = {
  to_order: 'Sob encomenda',
  stock: 'Estoque',
  in_house: 'Produção própria',
}

/** sourcingLabel is how the apps call a sourcing mode. */
export function sourcingLabel(mode: SourcingMode): string {
  return sourcing[mode]
}

/**
 * sourcingOptions are the modes "Origem" offers: Sob encomenda always, and
 * Estoque and Produção própria only when the store turned them on. The item's
 * own mode stays listed even if the store turned it off since.
 */
export function sourcingOptions(stockEnabled: boolean, productionEnabled: boolean, current: SourcingMode): SelectOption[] {
  const modes: SourcingMode[] = ['to_order']
  if (stockEnabled || current === 'stock') modes.push('stock')
  if (productionEnabled || current === 'in_house') modes.push('in_house')
  return modes.map((mode) => ({ value: mode, label: sourcing[mode] }))
}

/** isSourcingMode narrows a value from a select back to a sourcing mode. */
export function isSourcingMode(value: string): value is SourcingMode {
  return value in sourcing
}
