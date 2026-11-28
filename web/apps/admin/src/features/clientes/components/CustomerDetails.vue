<script setup lang="ts">
/**
 * CustomerDetails is the customer as labelled lines, for the roles that may
 * read customers but not change them (`customer:write`).
 */
import type { Address, Customer } from '@encomendo/api'

import { formatWhatsapp } from '../../../phone'

defineProps<{
  customer: Customer
}>()

/** oneLine writes an address the way a person would on an envelope. */
function oneLine(address: Address): string {
  const street = [address.street, address.number].filter((part) => part !== '').join(', ')
  const place = [address.complement, address.district].filter((part) => part !== '').join(' · ')
  const city = `${address.city} - ${address.state}`
  return [street, place, city, `CEP ${address.cep}`].filter((part) => part !== '').join(' · ')
}
</script>

<template>
  <dl class="details">
    <div>
      <dt>Nome</dt>
      <dd>{{ customer.name }}</dd>
    </div>
    <div>
      <dt>WhatsApp</dt>
      <dd>{{ formatWhatsapp(customer.phone) }}</dd>
    </div>
    <div>
      <dt>E-mail</dt>
      <dd>{{ customer.email ?? '—' }}</dd>
    </div>
    <div>
      <dt>Observações</dt>
      <dd class="notes">
        {{ customer.notes === '' ? '—' : customer.notes }}
      </dd>
    </div>
    <div>
      <dt>Endereços</dt>
      <dd v-if="customer.addresses.length === 0">
        —
      </dd>
      <dd
        v-for="address in customer.addresses"
        v-else
        :key="address.id"
      >
        <strong>{{ address.label }}</strong>: {{ oneLine(address) }}
      </dd>
    </div>
  </dl>
</template>

<style scoped>
.details {
  display: flex;
  flex-direction: column;
  gap: 12px;
  margin: 0;
}
dt {
  font-weight: 600;
}
dd {
  margin: 0;
}
.notes {
  white-space: pre-wrap;
}
</style>
