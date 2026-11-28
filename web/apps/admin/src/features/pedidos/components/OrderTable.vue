<script setup lang="ts">
/**
 * OrderTable is the order list: the number (a link to the order, in mono),
 * the customer, the status with its alert on top, "Itens prontos" as "a de
 * b", "Entrega prevista" and the total. The status words come from the
 * order's own workflow, the badge color from its phase. On the phone each
 * order is a card: number and status first, then the other lines.
 */
import type { OrderSummary } from '@encomendo/api'
import { StatusBadge } from '@encomendo/ui'
import { RouterLink } from 'vue-router'

import { formatCivilDate, formatMoney } from '../../../format'
import AlertBadge from '../../../components/AlertBadge.vue'
import { worstAlert } from '../labels'

defineProps<{
  items: readonly OrderSummary[]
}>()
</script>

<template>
  <table class="enc-cards">
    <caption class="enc-visually-hidden">
      Pedidos da loja
    </caption>
    <thead>
      <tr>
        <th scope="col">
          Pedido
        </th>
        <th scope="col">
          Cliente
        </th>
        <th scope="col">
          Status
        </th>
        <th
          scope="col"
          class="number"
        >
          Itens prontos
        </th>
        <th scope="col">
          Entrega prevista
        </th>
        <th
          scope="col"
          class="number"
        >
          Total
        </th>
      </tr>
    </thead>
    <tbody>
      <tr
        v-for="order in items"
        :key="order.id"
      >
        <th scope="row">
          <RouterLink
            class="code"
            :to="{ name: 'order', params: { orderId: order.id } }"
          >
            {{ order.number }}
          </RouterLink>
        </th>
        <td data-label="Cliente">
          {{ order.customer.name }}
        </td>
        <td
          class="status"
          data-label="Status"
        >
          <span class="badges">
            <StatusBadge
              :label="order.status.label"
              :phase="order.status.phase"
            />
            <AlertBadge
              v-if="worstAlert(order.alerts) !== null"
              :kind="worstAlert(order.alerts) ?? 'at_risk'"
            />
          </span>
        </td>
        <td
          class="number enc-number"
          data-label="Itens prontos"
        >
          {{ order.itemsReady }} de {{ order.itemsTotal }}
        </td>
        <td data-label="Entrega prevista">
          {{ formatCivilDate(order.deliveryDate) }}
        </td>
        <td
          class="number enc-number"
          data-label="Total"
        >
          {{ formatMoney(order.total.cents, order.total.currency) }}
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
.code {
  color: var(--enc-ink);
  font-family: var(--enc-font-mono);
  font-weight: 600;
}
.badges {
  display: inline-flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px;
}

/* Phone: the cards of enc-cards (tokens.css); the number is the card's title and a 48 px target. */
@media (max-width: 640px) {
  table.enc-cards tbody tr {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 8px 12px;
  }
  tbody th,
  td.status {
    grid-column: 1 / -1;
  }
  .number {
    text-align: left;
  }
  .code {
    display: flex;
    align-items: center;
    min-height: var(--enc-control-height);
  }
}
</style>
