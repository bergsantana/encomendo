import type { Item, ItemInput } from '@encomendo/api'

import { centsToText, parseCents } from '../../format'

/** SourcingMode is how the store gets an item: Sob encomenda, Estoque or Produção própria. */
export type SourcingMode = Item['sourcingMode']

/** SupplierDraft is one supplier of the item as the form holds it: texts, as typed. */
export interface SupplierDraft {
  /** supplierId is '' until one is chosen. */
  supplierId: string
  leadDays: string
  /** cost is the Custo field. It is only read when the owner is signed in. */
  cost: string
}

/** ChoiceDraft is one choice of a customer option. */
export interface ChoiceDraft {
  /** id is kept for a choice that already exists, so the server keeps it. */
  id?: string
  label: string
  extraPrice: string
  extraDays: string
}

/** OptionDraft is one "Opção do cliente". */
export interface OptionDraft {
  id?: string
  name: string
  required: boolean
  choices: ChoiceDraft[]
}

/**
 * ItemDraft is the item form's state. Everything a person types is text; it
 * becomes numbers only in `toItemInput`.
 */
export interface ItemDraft {
  name: string
  description: string
  /** categoryId is '' for "Sem categoria". */
  categoryId: string
  sourcingMode: SourcingMode
  price: string
  productionDays: string
  productionCost: string
  main: SupplierDraft
  /** backup is null until "Adicionar fornecedor reserva". */
  backup: SupplierDraft | null
  attributeValueIds: string[]
  options: OptionDraft[]
  published: boolean
  /**
   * stored remembers the costs the item had when it was read, text by
   * supplier (and for production), so only a cost the owner changed is sent.
   */
  stored: { productionCost: string; supplierCosts: Record<string, string> }
}

/** emptySupplier is a supplier slot with nothing chosen. */
export function emptySupplier(): SupplierDraft {
  return { supplierId: '', leadDays: '', cost: '' }
}

/** emptyDraft is the form of a new item. */
export function emptyDraft(): ItemDraft {
  return {
    name: '',
    description: '',
    categoryId: '',
    sourcingMode: 'to_order',
    price: '',
    productionDays: '',
    productionCost: '',
    main: emptySupplier(),
    backup: null,
    attributeValueIds: [],
    options: [],
    published: false,
    stored: { productionCost: '', supplierCosts: {} },
  }
}

function costText(cost: { cents: number } | null | undefined): string {
  return cost === null || cost === undefined ? '' : centsToText(cost.cents)
}

function supplierDraft(supplier: Item['mainSupplier']): SupplierDraft | null {
  if (supplier === null) return null
  return { supplierId: supplier.supplierId, leadDays: String(supplier.leadDays), cost: costText(supplier.cost) }
}

/** draftFromItem fills the form with an item read from the API. */
export function draftFromItem(item: Item): ItemDraft {
  const main = supplierDraft(item.mainSupplier)
  const backup = supplierDraft(item.backupSupplier)
  const supplierCosts: Record<string, string> = {}
  for (const supplier of [main, backup]) if (supplier !== null) supplierCosts[supplier.supplierId] = supplier.cost
  return {
    name: item.name,
    description: item.description,
    categoryId: item.categoryId ?? '',
    sourcingMode: item.sourcingMode,
    price: centsToText(item.price.cents),
    productionDays: item.productionDays === null ? '' : String(item.productionDays),
    productionCost: costText(item.productionCost),
    main: main ?? emptySupplier(),
    backup,
    attributeValueIds: [...item.attributeValueIds],
    options: item.options.map((option) => ({
      id: option.id,
      name: option.name,
      required: option.required,
      choices: option.choices.map((choice) => ({
        id: choice.id,
        label: choice.label,
        extraPrice: choice.extraPrice.cents === 0 ? '' : centsToText(choice.extraPrice.cents),
        extraDays: choice.extraDays === 0 ? '' : String(choice.extraDays),
      })),
    })),
    published: item.published,
    stored: { productionCost: costText(item.productionCost), supplierCosts },
  }
}

/** costOf is the cost text a supplier has on the stored item: '' for a supplier the item did not have. */
export function storedCost(draft: ItemDraft, supplierId: string): string {
  return draft.stored.supplierCosts[supplierId] ?? ''
}

/**
 * costField says what to send for a cost the owner may have changed: nothing
 * when the text is as stored (the server keeps its cost), `null` when it was
 * cleared, the cents otherwise. Text that is not an amount sends nothing; the
 * form refuses it before this runs.
 */
function costField(text: string, stored: string): { costCents?: number | null } {
  const typed = text.trim()
  if (typed === stored.trim()) return {}
  if (typed === '') return { costCents: null }
  const cents = parseCents(typed)
  return cents === null ? {} : { costCents: cents }
}

function wholeNumber(text: string): number {
  const typed = text.trim()
  return /^\d+$/.test(typed) ? Number(typed) : Number.NaN
}

function supplierInput(draft: ItemDraft, supplier: SupplierDraft, sendsCosts: boolean): NonNullable<ItemInput['mainSupplier']> {
  const base = { supplierId: supplier.supplierId, leadDays: wholeNumber(supplier.leadDays) }
  // ADR 0025: a role that is not the owner never sends a cost key, not even null.
  return sendsCosts ? { ...base, ...costField(supplier.cost, storedCost(draft, supplier.supplierId)) } : base
}

/**
 * toItemInput builds the body of an item write from the form. A cost key is
 * left out unless `sendsCosts` (the owner is signed in) and the owner changed
 * that cost; then it carries the cents, or null for a cleared field
 * (ADR 0025). Fields the sourcing mode does not use are sent as null.
 */
export function toItemInput(draft: ItemDraft, sendsCosts: boolean): ItemInput {
  const inHouse = draft.sourcingMode === 'in_house'
  const input: ItemInput = {
    name: draft.name.trim(),
    description: draft.description.trim(),
    categoryId: draft.categoryId === '' ? null : draft.categoryId,
    sourcingMode: draft.sourcingMode,
    priceCents: parseCents(draft.price) ?? Number.NaN,
    productionDays: inHouse ? wholeNumber(draft.productionDays) : null,
    mainSupplier: inHouse ? null : supplierInput(draft, draft.main, sendsCosts),
    backupSupplier: inHouse || draft.backup === null ? null : supplierInput(draft, draft.backup, sendsCosts),
    attributeValueIds: draft.attributeValueIds,
    options: draft.options.map((option) => ({
      ...(option.id === undefined ? {} : { id: option.id }),
      name: option.name.trim(),
      required: option.required,
      choices: option.choices.map((choice) => ({
        ...(choice.id === undefined ? {} : { id: choice.id }),
        label: choice.label.trim(),
        extraPriceCents: choice.extraPrice.trim() === '' ? 0 : (parseCents(choice.extraPrice) ?? Number.NaN),
        extraDays: choice.extraDays.trim() === '' ? 0 : wholeNumber(choice.extraDays),
      })),
    })),
    published: draft.published,
  }
  if (inHouse && sendsCosts) {
    const production = costField(draft.productionCost, draft.stored.productionCost)
    if (production.costCents !== undefined) input.productionCostCents = production.costCents
  }
  return input
}

/** FormCheck is one thing to check before a save: the field key, the pt-BR message and whether it is right. */
export interface FormCheck {
  field: string
  message: string
  ok: boolean
}

const isDays = (text: string): boolean => {
  const days = wholeNumber(text)
  return Number.isInteger(days) && days >= 0 && days <= 365
}
const isProductionDays = (text: string): boolean => {
  const days = wholeNumber(text)
  return Number.isInteger(days) && days >= 1 && days <= 365
}
const isMoneyOrBlank = (text: string): boolean => text.trim() === '' || parseCents(text) !== null

/**
 * itemChecks lists what the form can tell is wrong before calling the API,
 * with the wording of the design document. The server checks the same rules
 * again and its messages show too.
 */
export function itemChecks(draft: ItemDraft, sendsCosts: boolean): FormCheck[] {
  const checks: FormCheck[] = [
    { field: 'name', message: 'Informe o nome do item.', ok: draft.name.trim() !== '' },
    { field: 'price', message: 'Informe o preço.', ok: parseCents(draft.price) !== null },
  ]
  if (draft.sourcingMode === 'in_house') {
    checks.push({
      field: 'productionDays',
      message: draft.productionDays.trim() === '' ? 'Informe os dias de produção.' : 'Use de 1 a 365 dias de produção.',
      ok: isProductionDays(draft.productionDays),
    })
    if (sendsCosts) checks.push({ field: 'productionCost', message: 'Informe um custo de zero ou mais.', ok: isMoneyOrBlank(draft.productionCost) })
  } else {
    checks.push(...supplierChecks('main', draft.main, sendsCosts, 'Escolha o fornecedor principal.'))
    if (draft.backup !== null) {
      checks.push(...supplierChecks('backup', draft.backup, sendsCosts, 'Escolha o fornecedor reserva.'))
      if (draft.backup.supplierId !== '' && draft.backup.supplierId === draft.main.supplierId) {
        checks.push({ field: 'backupSupplier', message: 'O fornecedor reserva precisa ser outro.', ok: false })
      }
    }
  }
  draft.options.forEach((option, index) => {
    checks.push({ field: `options.${index}.name`, message: 'Informe o nome da opção.', ok: option.name.trim() !== '' })
    checks.push({ field: `options.${index}.choices`, message: 'Adicione ao menos uma escolha.', ok: option.choices.length > 0 })
    const seen = new Set<string>()
    option.choices.forEach((choice, at) => {
      const key = choice.label.trim().toLowerCase()
      const duplicate = key !== '' && seen.has(key)
      seen.add(key)
      checks.push({ field: `options.${index}.choices.${at}.label`, message: duplicate ? 'Já existe uma escolha com este nome.' : 'Informe a escolha.', ok: key !== '' && !duplicate })
    })
  })
  return checks
}

function supplierChecks(slot: 'main' | 'backup', supplier: SupplierDraft, sendsCosts: boolean, missing: string): FormCheck[] {
  const checks: FormCheck[] = [
    { field: `${slot}Supplier`, message: missing, ok: supplier.supplierId !== '' },
    { field: `${slot}LeadDays`, message: supplier.leadDays.trim() === '' ? 'Informe o prazo do fornecedor.' : 'Use um prazo de 0 a 365 dias.', ok: isDays(supplier.leadDays) },
  ]
  if (sendsCosts) checks.push({ field: `${slot}Cost`, message: 'Informe um custo de zero ou mais.', ok: isMoneyOrBlank(supplier.cost) })
  return checks
}

/**
 * formField maps a field name the API reports (a path into the request body,
 * such as `mainSupplier.leadDays` or `options[0].name`) to the key of the form
 * input that shows it. A name the form has no input for is kept as is.
 */
export function formField(apiField: string): string {
  const path = apiField.replace(/\[(\d+)\]/g, '.$1')
  const direct: Record<string, string> = {
    priceCents: 'price',
    productionCostCents: 'productionCost',
    mainSupplier: 'mainSupplier',
    'mainSupplier.supplierId': 'mainSupplier',
    'mainSupplier.leadDays': 'mainLeadDays',
    'mainSupplier.costCents': 'mainCost',
    backupSupplier: 'backupSupplier',
    'backupSupplier.supplierId': 'backupSupplier',
    'backupSupplier.leadDays': 'backupLeadDays',
    'backupSupplier.costCents': 'backupCost',
  }
  const mapped = direct[path]
  if (mapped !== undefined) return mapped
  return path.replace(/\.extraPriceCents$/, '.extraPrice')
}
