import type { Customer, CustomerSummary, Order, OrderStatus, OrderSummary, StatusWorkflow, TenantSettings } from '@encomendo/api'
import { HttpResponse } from 'msw'

/**
 * Fixtures for the M4 screens: two status workflows with different words, one
 * order seen as the owner and as an admin (the owner's carries cost and
 * margin), customers, and the answer to every problem code the order calls
 * can raise. Tests build their handlers from these so no screen depends on
 * one preset's labels.
 */

export const brl = (cents: number) => ({ cents, currency: 'BRL' })

const id = (n: number) => `0190f3c4-5e1a-7b2c-8d3e-${String(n).padStart(12, '0')}`

/** encomendaMontagem is the "Encomenda com montagem" preset, with an assembly step. */
export const encomendaMontagem: StatusWorkflow = {
  id: id(9001),
  version: 1,
  key: 'encomenda_montagem',
  name: 'Encomenda com montagem',
  statuses: [
    { key: 'rascunho', label: 'Rascunho', clientLabel: 'Pedido recebido', phase: 'draft' },
    { key: 'aguardando_sinal', label: 'Aguardando sinal', clientLabel: 'Aguardando o sinal', phase: 'awaiting_deposit' },
    { key: 'confirmado', label: 'Confirmado', clientLabel: 'Pedido confirmado', phase: 'confirmed' },
    { key: 'comprando', label: 'Comprando peças', clientLabel: 'Separando as peças', phase: 'sourcing' },
    { key: 'em_montagem', label: 'Em montagem', clientLabel: 'Montando o seu pedido', phase: 'assembling' },
    { key: 'pronto', label: 'Pronto', clientLabel: 'Pronto para retirar ou receber', phase: 'ready' },
    { key: 'entregue', label: 'Entregue', clientLabel: 'Entregue', phase: 'handed_over' },
  ],
}

/** prontaEntrega is the "Pronta entrega" preset: no assembly, other words for the same phases. */
export const prontaEntrega: StatusWorkflow = {
  id: id(9002),
  version: 1,
  key: 'pronta_entrega',
  name: 'Pronta entrega',
  statuses: [
    { key: 'orcamento', label: 'Orçamento', clientLabel: 'Recebemos o seu pedido', phase: 'draft' },
    { key: 'pagar_sinal', label: 'Pagar sinal', clientLabel: 'Falta o sinal', phase: 'awaiting_deposit' },
    { key: 'separando', label: 'Separando', clientLabel: 'Estamos separando', phase: 'sourcing' },
    { key: 'separado', label: 'Separado', clientLabel: 'Separado para você', phase: 'ready' },
    { key: 'retirado', label: 'Retirado', clientLabel: 'Retirado', phase: 'handed_over' },
  ],
}

/** workflows are the presets the API lists, in order. */
export const workflows: StatusWorkflow[] = [encomendaMontagem, prontaEntrega]

/** statusOf returns the status of a workflow by key; it throws when a test asks for one that is not there. */
export function statusOf(workflow: StatusWorkflow, key: string): OrderStatus {
  const found = workflow.statuses.find((status) => status.key === key)
  if (found === undefined) throw new Error(`no status ${key} in ${workflow.key}`)
  return found
}

/** settingsUsing is the store's settings with the given workflow in use. */
export function settingsUsing(workflow: StatusWorkflow, overrides: Partial<TenantSettings> = {}): TenantSettings {
  return {
    stockEnabled: false,
    productionEnabled: false,
    purchaseBeforeDeposit: false,
    depositPercent: 50,
    assemblyDays: 3,
    safetyMarginDays: 2,
    whatsappPhone: null,
    timeZone: 'America/Sao_Paulo',
    currency: 'BRL',
    statusWorkflow: { id: workflow.id, version: workflow.version, name: workflow.name, statuses: workflow.statuses },
    version: 7,
    ...overrides,
  }
}

export const joao: Customer = {
  id: id(101),
  name: 'João Pereira',
  phone: '5592991234567',
  email: 'joao@example.com',
  notes: 'Prefere retirar na loja.',
  addresses: [
    {
      id: id(111),
      label: 'Casa',
      cep: '69057000',
      street: 'Rua das Palmeiras',
      number: '120',
      complement: 'Apto 4',
      district: 'Adrianópolis',
      city: 'Manaus',
      state: 'AM',
    },
  ],
  version: 3,
}

export const maria: Customer = { id: id(102), name: 'Maria Souza', phone: '5592988887777', email: null, notes: '', addresses: [], version: 1 }

/** summaryOf is the list row of a customer. */
export function summaryOf(customer: Customer, extra: Partial<CustomerSummary> = {}): CustomerSummary {
  return { id: customer.id, name: customer.name, phone: customer.phone, orders: 0, ...extra }
}

/** orderSummary is one row of the order list, in the given workflow. */
export function orderSummary(workflow: StatusWorkflow, statusKey: string, overrides: Partial<OrderSummary> = {}): OrderSummary {
  return {
    id: id(201),
    number: 'P-0231',
    customer: { id: joao.id, name: joao.name },
    status: statusOf(workflow, statusKey),
    itemsReady: 1,
    itemsTotal: 3,
    deliveryDate: '2026-12-12',
    total: brl(129990),
    balanceDue: brl(64995),
    alerts: [],
    ...overrides,
  }
}

/** orderAsAdmin is a full order without cost or margin, as every role but the owner receives it. */
export function orderAsAdmin(workflow: StatusWorkflow = encomendaMontagem, statusKey = 'aguardando_sinal', overrides: Partial<Order> = {}): Order {
  const itemId = id(301)
  return {
    id: id(201),
    number: 'P-0231',
    customer: { id: joao.id, name: joao.name, phone: joao.phone, whatsappUrl: `https://wa.me/${joao.phone}` },
    status: statusOf(workflow, statusKey),
    workflow,
    moves: workflow.statuses.filter((status) => status.key !== statusKey),
    lines: [{ id: id(401), kit: null, price: brl(129990), itemIds: [itemId] }],
    items: [
      {
        id: itemId,
        lineId: id(401),
        catalogItemId: id(501),
        name: 'Banco de couro Scrambler',
        sourcingMode: 'to_order',
        quantity: 1,
        unitPrice: brl(129990),
        options: [],
        status: 'to_buy',
        check: null,
        expectedDate: '2026-12-05',
        arrivedOn: null,
        arrived: false,
        alerts: [],
        nextMoves: ['sent_to_supplier'],
      },
    ],
    handover: 'pickup',
    address: null,
    subtotal: brl(129990),
    freight: brl(0),
    discount: brl(0),
    total: brl(129990),
    depositPercent: 50,
    depositDue: brl(64995),
    depositOwed: brl(64995),
    depositPaid: false,
    paid: brl(0),
    balanceDue: brl(129990),
    refundDue: brl(0),
    released: false,
    payments: [],
    releaseDate: null,
    deliveryDate: '2026-12-12',
    deliveryDateChanged: false,
    arrivalLimit: '2026-12-09',
    assemblyDays: 3,
    safetyMarginDays: 2,
    editable: 'free',
    createdAt: '2026-11-27T12:00:00Z',
    version: 1,
    ...overrides,
  }
}

/** orderAsOwner is the same order as the owner receives it: with the cost and the margin the other roles never get. */
export function orderAsOwner(workflow: StatusWorkflow = encomendaMontagem, statusKey = 'aguardando_sinal', overrides: Partial<Order> = {}): Order {
  const base = orderAsAdmin(workflow, statusKey, overrides)
  const [first, ...rest] = base.items
  return {
    ...base,
    items: first === undefined ? base.items : [{ ...first, cost: brl(90000) }, ...rest],
    cost: brl(90000),
    margin: { cents: 39990, basisPoints: 3076 },
  }
}

/**
 * problemCodes are the refusals the order and customer calls can answer, with
 * the status and pt-BR title each carries. `orderProblem(code)` builds the response.
 */
export const problemCodes = {
  invalid: [400, 'Confira os campos destacados.'],
  approval_required: [400, 'Confirme que o cliente aprovou a mudança.'],
  payment_exceeds_due: [400, 'O valor passa do que falta pagar.'],
  idempotency_key_required: [400, 'Falta a chave de idempotência.'],
  not_found: [404, 'Não encontrado.'],
  version_conflict: [409, 'Outra pessoa alterou este pedido. Recarregue para ver a versão atual.'],
  conflict: [409, 'Esta ação ainda está sendo processada.'],
  invalid_transition: [409, 'Este pedido não pode ir para este status.'],
  guard_failed: [409, 'O pedido não pode avançar ainda.'],
  order_not_editable: [409, 'Este pedido já está em montagem e não pode mais ser alterado.'],
  order_closed: [409, 'Este pedido já foi entregue ou cancelado.'],
  invalid_item_move: [409, 'O item não pode ir para este estado.'],
  no_backup_supplier: [409, 'Este item não tem fornecedor reserva.'],
  item_unavailable: [409, 'Há item arquivado no pedido.'],
  idempotency_key_reused: [422, 'Esta chave já foi usada em outro pedido.'],
  phone_taken: [409, 'Já existe um cliente com este WhatsApp.'],
} as const

/** ProblemCode is a code `orderProblem` knows. */
export type ProblemCode = keyof typeof problemCodes

/** orderProblem answers like the API does with the given code; `extra` adds fields such as `guard` or `references`. */
export function orderProblem(code: ProblemCode, extra: Record<string, unknown> = {}): HttpResponse<string> {
  const [status, title] = problemCodes[code]
  return new HttpResponse(JSON.stringify({ type: 'about:blank', title, status, code, ...extra }), {
    status,
    headers: { 'Content-Type': 'application/problem+json' },
  })
}
