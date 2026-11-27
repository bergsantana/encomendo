import type { Kit, KitInput } from '@encomendo/api'

import { centsToText, parseCents } from '../../format'
import { type FormCheck, wholeNumber } from './itemForm'

/** SwapDiscount is what a client's swap does to the kit price: "Manter o desconto do kit" (keep) or "Cobrar a soma dos itens avulsos" (lose). */
export type SwapDiscount = Kit['swapDiscount']

/** KitItemDraft is one row of "Itens do kit" as the form holds it. */
export interface KitItemDraft {
  itemId: string
  itemName: string
  /** quantity is typed text, a whole number from 1 to 99 once checked. */
  quantity: string
  swappable: boolean
  /** alternativeItemIds are the "Pode trocar por" choices; sent only while `swappable`. */
  alternativeItemIds: string[]
  /** archived marks an item archived after the kit was saved; the API refuses to save it again. */
  archived: boolean
}

/**
 * KitDraft is the kit form's state. Everything a person types is text; it
 * becomes numbers only in `toKitInput`. It holds no totals or days: those
 * come from the API after a save, never from the form.
 */
export interface KitDraft {
  name: string
  description: string
  price: string
  swapDiscount: SwapDiscount
  /** assemblyDays is '' to use the store's default. */
  assemblyDays: string
  /** safetyMarginDays is '' to use the store's default. */
  safetyMarginDays: string
  items: KitItemDraft[]
  attributeValueIds: string[]
  published: boolean
}

/** emptyKitDraft is the form of a new kit: it keeps the discount on a swap, as ADR 0023 says. */
export function emptyKitDraft(): KitDraft {
  return {
    name: '',
    description: '',
    price: '',
    swapDiscount: 'keep',
    assemblyDays: '',
    safetyMarginDays: '',
    items: [],
    attributeValueIds: [],
    published: false,
  }
}

function daysText(days: number | null): string {
  return days === null ? '' : String(days)
}

/** draftFromKit fills the form with a kit read from the API. */
export function draftFromKit(kit: Kit): KitDraft {
  return {
    name: kit.name,
    description: kit.description,
    price: centsToText(kit.price.cents),
    swapDiscount: kit.swapDiscount,
    assemblyDays: daysText(kit.assemblyDays),
    safetyMarginDays: daysText(kit.safetyMarginDays),
    items: kit.items.map((row) => ({
      itemId: row.itemId,
      itemName: row.itemName,
      quantity: String(row.quantity),
      swappable: row.swappable,
      alternativeItemIds: [...row.alternativeItemIds],
      archived: row.archived,
    })),
    attributeValueIds: [...kit.attributeValueIds],
    published: kit.published,
  }
}

function overrideDays(text: string): number | null {
  return text.trim() === '' ? null : wholeNumber(text)
}

/**
 * toKitInput builds the body of a kit write from the form. A blank days field
 * is sent as null (the store's default); a number is the kit's own. A row that
 * is not swappable sends no alternatives, as the contract asks.
 */
export function toKitInput(draft: KitDraft): KitInput {
  return {
    name: draft.name.trim(),
    description: draft.description.trim(),
    priceCents: parseCents(draft.price) ?? Number.NaN,
    swapDiscount: draft.swapDiscount,
    assemblyDays: overrideDays(draft.assemblyDays),
    safetyMarginDays: overrideDays(draft.safetyMarginDays),
    items: draft.items.map((row) => ({
      itemId: row.itemId,
      quantity: wholeNumber(row.quantity),
      swappable: row.swappable,
      alternativeItemIds: row.swappable ? row.alternativeItemIds : [],
    })),
    attributeValueIds: draft.attributeValueIds,
    published: draft.published,
  }
}

const isOverrideDays = (text: string): boolean => {
  if (text.trim() === '') return true
  const days = wholeNumber(text)
  return Number.isInteger(days) && days >= 0 && days <= 60
}

const isQuantity = (text: string): boolean => {
  const quantity = wholeNumber(text)
  return Number.isInteger(quantity) && quantity >= 1 && quantity <= 99
}

/**
 * kitChecks lists what the form can tell is wrong before calling the API,
 * with the wording of the design document. The server checks the same rules
 * again and its messages show too.
 */
export function kitChecks(draft: KitDraft): FormCheck[] {
  const cents = parseCents(draft.price)
  const checks: FormCheck[] = [
    { field: 'name', message: 'Informe o nome do kit.', ok: draft.name.trim() !== '' },
    { field: 'price', message: 'Informe o preço do kit.', ok: cents !== null && cents > 0 },
    { field: 'items', message: 'Adicione ao menos um item.', ok: draft.items.length > 0 },
    { field: 'assemblyDays', message: 'Use de 0 a 60 dias.', ok: isOverrideDays(draft.assemblyDays) },
    { field: 'safetyMarginDays', message: 'Use de 0 a 60 dias.', ok: isOverrideDays(draft.safetyMarginDays) },
  ]
  draft.items.forEach((row, index) => {
    checks.push({ field: `items.${index}.quantity`, message: 'Use uma quantidade de 1 a 99.', ok: isQuantity(row.quantity) })
    checks.push({ field: `items.${index}.alternatives`, message: 'Escolha ao menos um item para a troca.', ok: !row.swappable || row.alternativeItemIds.length > 0 })
  })
  return checks
}

/**
 * kitField maps a field name the API reports (a path into the request body,
 * such as `priceCents` or `items[0].quantity`) to the key of the form input
 * that shows it. A name the form has no input for is kept as is.
 */
export function kitField(apiField: string): string {
  const path = apiField.replace(/\[(\d+)\]/g, '.$1')
  if (path === 'priceCents') return 'price'
  return path.replace(/\.alternativeItemIds(\.\d+)?$/, '.alternatives')
}
