import type { AddressInput, Customer, CustomerInput, UF } from '@encomendo/api'

import type { FieldCheck } from '../../composables/useForm'
import { formatWhatsapp, isWhatsappNumber, whatsappDigits } from '../../phone'

/** AddressDraft is one address as it is edited; `id` is kept for a saved one so orders pointing at it still do. */
export interface AddressDraft {
  id?: string
  label: string
  cep: string
  street: string
  number: string
  complement: string
  district: string
  city: string
  /** state is '' until a UF is chosen. */
  state: UF | ''
}

/** CustomerDraft is the customer form as it is edited: the WhatsApp number as typed, the CEP with or without its dash. */
export interface CustomerDraft {
  name: string
  phone: string
  email: string
  notes: string
  addresses: AddressDraft[]
}

/** maxAddresses is how many addresses a customer may have (the API refuses more). */
export const maxAddresses = 10

/** ufs are the 27 states the address form offers. */
export const ufs: readonly UF[] = [
  'AC', 'AL', 'AP', 'AM', 'BA', 'CE', 'DF', 'ES', 'GO', 'MA', 'MT', 'MS', 'MG', 'PA', 'PB', 'PR', 'PE', 'PI', 'RJ', 'RN', 'RS', 'RO', 'RR', 'SC', 'SP', 'SE', 'TO',
]

/** isUF narrows what a select gave back to a state code. */
export function isUF(value: string): value is UF {
  return (ufs as readonly string[]).includes(value)
}

/** emptyAddress is the "Adicionar endereço" starting point. */
export function emptyAddress(): AddressDraft {
  return { label: '', cep: '', street: '', number: '', complement: '', district: '', city: '', state: '' }
}

/** emptyDraft is the form of "Novo cliente". */
export function emptyDraft(): CustomerDraft {
  return { name: '', phone: '', email: '', notes: '', addresses: [] }
}

/** draftOf is the form for a customer being edited, or the empty one of "Novo cliente". */
export function draftOf(customer: Customer | null): CustomerDraft {
  return customer === null ? emptyDraft() : draftFromCustomer(customer)
}

/** draftFromCustomer fills the form from a customer the API sent. */
export function draftFromCustomer(customer: Customer): CustomerDraft {
  return {
    name: customer.name,
    phone: formatWhatsapp(customer.phone),
    email: customer.email ?? '',
    notes: customer.notes,
    addresses: customer.addresses.map((address) => ({
      id: address.id,
      label: address.label,
      cep: address.cep,
      street: address.street,
      number: address.number,
      complement: address.complement,
      district: address.district,
      city: address.city,
      state: address.state,
    })),
  }
}

/** addressField names an address field the way the API reports it, "addresses[2].cep". */
export function addressField(index: number, field: keyof AddressInput): string {
  return `addresses[${index}].${field}`
}

const emailShape = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

/** within passes text of at most `max` characters; an empty one is for the required check to judge. */
function within(max: number): (text: string) => boolean {
  return (text) => text.length <= max
}

function addressChecks(address: AddressDraft, index: number): FieldCheck[] {
  const at = (field: keyof AddressInput) => addressField(index, field)
  // Limits are those of AddressInput in docs/api/openapi.yaml.
  return [
    [at('label'), address.label.trim(), 'Informe o nome do endereço.'],
    [at('label'), address.label.trim(), 'Use um nome de endereço de até 40 caracteres.', within(40)],
    [at('cep'), address.cep, 'Confira o CEP: use 8 dígitos.', (cep) => cep.replace(/\D/g, '').length === 8],
    [at('street'), address.street.trim(), 'Informe a rua.'],
    [at('street'), address.street.trim(), 'Use uma rua de até 200 caracteres.', within(200)],
    [at('number'), address.number.trim(), 'Use um número de até 20 caracteres.', within(20)],
    [at('complement'), address.complement.trim(), 'Use um complemento de até 100 caracteres.', within(100)],
    [at('district'), address.district.trim(), 'Use um bairro de até 100 caracteres.', within(100)],
    [at('city'), address.city.trim(), 'Informe a cidade.'],
    [at('city'), address.city.trim(), 'Use uma cidade de até 100 caracteres.', within(100)],
    [at('state'), address.state, 'Escolha a UF.'],
  ]
}

/**
 * customerChecks are the checks run before the call, in the words the API
 * uses; the API checks the same rules and the lengths again.
 */
export function customerChecks(draft: CustomerDraft): FieldCheck[] {
  return [
    ['name', draft.name.trim(), 'Informe o nome do cliente.'],
    ['name', draft.name.trim(), 'Use um nome de até 120 caracteres.', (name) => name.length <= 120],
    ['phone', draft.phone.trim(), 'Informe o WhatsApp do cliente.'],
    ['phone', draft.phone, 'Confira o número: use DDD e 8 ou 9 dígitos.', (phone) => phone.trim() === '' || isWhatsappNumber(phone)],
    ['email', draft.email.trim(), 'Confira o e-mail.', (email) => email === '' || emailShape.test(email)],
    ['notes', draft.notes, 'Use observações de até 2000 caracteres.', (notes) => notes.length <= 2000],
    ['addresses', String(draft.addresses.length), 'Use até 10 endereços.', (count) => Number(count) <= maxAddresses],
    ...draft.addresses.flatMap(addressChecks),
  ]
}

function toAddressInput(address: AddressDraft): AddressInput {
  if (address.state === '') throw new Error('an address without a state passed its checks')
  return {
    ...(address.id === undefined ? {} : { id: address.id }),
    label: address.label.trim(),
    cep: address.cep.replace(/\D/g, ''),
    street: address.street.trim(),
    number: address.number.trim(),
    complement: address.complement.trim(),
    district: address.district.trim(),
    city: address.city.trim(),
    state: address.state,
  }
}

/** toCustomerInput builds the body of the save, with the number as digits; an empty e-mail is sent as null. */
export function toCustomerInput(draft: CustomerDraft): CustomerInput {
  return {
    name: draft.name.trim(),
    phone: whatsappDigits(draft.phone),
    email: draft.email.trim() === '' ? null : draft.email.trim(),
    notes: draft.notes.trim(),
    addresses: draft.addresses.map(toAddressInput),
  }
}
