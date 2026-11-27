import type { Attribute, Category, Item, ItemSummary, Permission, Supplier, TenantSettings } from '@encomendo/api'
import type { VueWrapper } from '@vue/test-utils'
import { HttpResponse } from 'msw'

import { field as fieldByLabel } from '../../testing/app'

// Fixtures and queries the Catálogo tests share: the Itens and Kits specs read
// the same store, so they describe it the same way.

export const readOnly: Permission[] = ['catalog:read', 'settings:read']
export const writer: Permission[] = ['catalog:read', 'catalog:write', 'settings:read']

export const bancos: Category = { id: '0190f3c4-5e1a-7b2c-8d3e-0000000000c1', name: 'Bancos', position: 0 }
export const guidoes: Category = { id: '0190f3c4-5e1a-7b2c-8d3e-0000000000c2', name: 'Guidões', position: 1 }
export const couros: Supplier = { id: '0190f3c4-5e1a-7b2c-8d3e-0000000000f1', name: 'Couros Paulista', whatsappPhone: '5511999990000', version: 1 }
export const metais: Supplier = { id: '0190f3c4-5e1a-7b2c-8d3e-0000000000f2', name: 'Metais Sul', whatsappPhone: '5551999990000', version: 1 }
export const serveEm: Attribute = {
  id: '0190f3c4-5e1a-7b2c-8d3e-0000000000a1',
  name: 'Serve em',
  position: 0,
  values: [
    { id: '0190f3c4-5e1a-7b2c-8d3e-0000000000b1', label: 'CB 500X', usageCount: 3 },
    { id: '0190f3c4-5e1a-7b2c-8d3e-0000000000b2', label: 'XRE 300', usageCount: 0 },
  ],
}

export const brl = (cents: number) => ({ cents, currency: 'BRL' })
export const bancoId = '0190f3c4-5e1a-7b2c-8d3e-000000000101'

export function summary(overrides: Partial<ItemSummary> = {}): ItemSummary {
  return {
    id: bancoId,
    name: 'Banco de couro Scrambler',
    categoryId: bancos.id,
    sourcingMode: 'to_order',
    price: brl(45000),
    readyDays: 15,
    mainSupplierName: couros.name,
    published: true,
    archived: false,
    cover: null,
    version: 3,
    ...overrides,
  }
}

/** costs are what only the owner's responses carry (ADR 0025). */
export const ownerCosts = { cost: brl(31500), margin: { cents: 13500, basisPoints: 3000 } }

export function item(overrides: Partial<Item> = {}): Item {
  return {
    id: bancoId,
    name: 'Banco de couro Scrambler',
    description: 'Couro sintético',
    categoryId: bancos.id,
    sourcingMode: 'to_order',
    price: brl(45000),
    productionDays: null,
    readyDays: 15,
    mainSupplier: { supplierId: couros.id, supplierName: couros.name, leadDays: 15 },
    backupSupplier: null,
    attributeValueIds: [],
    options: [],
    photos: [],
    published: true,
    archived: false,
    version: 3,
    ...overrides,
  }
}

export const ownerItem = () =>
  item({
    mainSupplier: { supplierId: couros.id, supplierName: couros.name, leadDays: 15, cost: brl(31500) },
    ...ownerCosts,
  })

export function settings(overrides: Partial<TenantSettings> = {}): TenantSettings {
  return {
    stockEnabled: false,
    productionEnabled: false,
    purchaseBeforeDeposit: false,
    depositPercent: 50,
    assemblyDays: 3,
    safetyMarginDays: 2,
    whatsappPhone: null,
    currency: 'BRL',
    statusWorkflow: { id: '0190f3c4-5e1a-7b2c-8d3e-4f5a6b7c8d01', version: 1, name: 'Encomenda com montagem' },
    version: 1,
    ...overrides,
  }
}

export function problem(status: number, code: string, title: string, extra: Record<string, unknown> = {}) {
  return HttpResponse.json({ type: 'about:blank', title, status, code, ...extra }, { status, headers: { 'Content-Type': 'application/problem+json' } })
}

export type Scope = Pick<VueWrapper, 'findAll' | 'find'>

/** button finds a button by its visible text, or by the name a screen reader reads when that is longer. */
export function button(wrapper: Scope, name: string) {
  const found = wrapper.findAll('button').find((b) => b.text() === name || b.attributes('aria-label') === name)
  if (found === undefined) throw new Error(`no button ${name}`)
  return found
}

/** columnHeaders reads the table's column titles. */
export function columnHeaders(wrapper: Scope): string[] {
  return wrapper.findAll('thead th').map((th) => th.text())
}

/** field finds a control by its label, its aria-label, or the label that wraps it (a checkbox). */
export function field(wrapper: Scope, label: string) {
  const named = wrapper.find(`[aria-label="${label}"]`)
  if (named.exists()) return named
  const wrapping = wrapper.findAll('label').find((l) => l.text() === label && l.attributes('for') === undefined)
  if (wrapping !== undefined) return wrapping.get('input')
  return fieldByLabel(wrapper as VueWrapper, label)
}

export const plain = (text: string) => text.replace(/\u00a0/g, ' ')

/** group finds a fieldset by its legend, such as "Fornecedor reserva". */
export function group(wrapper: { findAll: VueWrapper['findAll'] }, legend: string) {
  const found = wrapper.findAll('fieldset').find((fieldset) => fieldset.find('legend')?.text() === legend)
  if (found === undefined) throw new Error(`no group ${legend}`)
  return found
}

