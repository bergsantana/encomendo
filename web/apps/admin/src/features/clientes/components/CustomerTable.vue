<script setup lang="ts">
/**
 * CustomerTable is the customer list: name (a link that opens the customer),
 * WhatsApp, how many orders, and the last one's date. On the phone each row
 * is a card with the same lines.
 */
import type { CustomerSummary } from '@encomendo/api'
import { RouterLink } from 'vue-router'

import { formatDate } from '../../../format'
import { formatWhatsapp } from '../../../phone'

defineProps<{
  items: readonly CustomerSummary[]
  /** timeZone is the store's, so "Último pedido" shows the day the store lived it. */
  timeZone?: string
}>()
</script>

<template>
  <table class="enc-cards">
    <caption class="enc-visually-hidden">
      Clientes da loja
    </caption>
    <thead>
      <tr>
        <th scope="col">
          Nome
        </th>
        <th scope="col">
          WhatsApp
        </th>
        <th
          scope="col"
          class="number"
        >
          Pedidos
        </th>
        <th scope="col">
          Último pedido
        </th>
      </tr>
    </thead>
    <tbody>
      <tr
        v-for="customer in items"
        :key="customer.id"
      >
        <th scope="row">
          <RouterLink
            class="name"
            :to="{ name: 'customer', params: { customerId: customer.id } }"
          >
            {{ customer.name }}
          </RouterLink>
        </th>
        <td data-label="WhatsApp">
          {{ formatWhatsapp(customer.phone) }}
        </td>
        <td
          class="number enc-number"
          data-label="Pedidos"
        >
          {{ customer.orders }}
        </td>
        <td data-label="Último pedido">
          {{ customer.lastOrderAt === undefined ? '—' : formatDate(customer.lastOrderAt, timeZone) }}
        </td>
      </tr>
    </tbody>
  </table>
</template>

<style scoped>
table {
  width: 100%;
  border-collapse: collapse;
  font-family: var(--enc-font-condensed);
  font-size: var(--enc-table-size, 13px);
}
th,
td {
  padding: 8px 12px;
  border-bottom: 1px solid var(--enc-hairline);
  text-align: left;
}
thead th {
  color: var(--enc-ink-secondary);
}
tbody th {
  font-weight: 400;
}
.number {
  text-align: right;
}
.name {
  color: var(--enc-ink);
  font-weight: 600;
}

/* Phone: the cards of enc-cards (tokens.css); the name is the card's title and a 48 px target. */
@media (max-width: 640px) {
  table.enc-cards tbody tr {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 8px 12px;
  }
  tbody th {
    grid-column: 1 / -1;
  }
  .number {
    text-align: left;
  }
  .name {
    display: flex;
    align-items: center;
    min-height: var(--enc-control-height);
  }
}
</style>
