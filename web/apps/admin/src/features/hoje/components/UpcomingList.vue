<script setup lang="ts">
/**
 * UpcomingList is "Próximas entregas": the active orders by delivery date, in
 * the server's order. The status words come from each order's own workflow
 * and its color from the phase; an alert sits on top of the status. On the
 * phone each order is a card.
 */
import type { OrderSummary } from '@encomendo/api'
import { StatusBadge } from '@encomendo/ui'
import { RouterLink } from 'vue-router'

import { formatCivilDate } from '../../../format'
import AlertBadge from '../../../components/AlertBadge.vue'

defineProps<{
  orders: readonly OrderSummary[]
}>()

/** kinds lists each alert kind once, whatever the number of items it is about. */
function kinds(order: OrderSummary): ('late' | 'at_risk')[] {
  return [...new Set(order.alerts.map((alert) => alert.kind))]
}
</script>

<template>
  <section
    class="upcoming"
    aria-labelledby="upcoming-deliveries"
  >
    <h2 id="upcoming-deliveries">
      Próximas entregas
    </h2>
    <p
      v-if="orders.length === 0"
      class="muted"
    >
      Nenhuma entrega prevista.
    </p>
    <table
      v-else
      class="enc-cards"
    >
      <caption class="enc-visually-hidden">
        Próximas entregas
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
          <th scope="col">
            Itens prontos
          </th>
          <th scope="col">
            Entrega
          </th>
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="order in orders"
          :key="order.id"
        >
          <th scope="row">
            <RouterLink
              class="number enc-code"
              :to="{ name: 'order', params: { orderId: order.id } }"
            >
              {{ order.number }}
            </RouterLink>
          </th>
          <td data-label="Cliente">
            {{ order.customer.name }}
          </td>
          <td data-label="Status">
            <span class="status">
              <StatusBadge
                :label="order.status.label"
                :phase="order.status.phase"
              />
              <AlertBadge
                v-for="kind in kinds(order)"
                :key="kind"
                :kind="kind"
              />
            </span>
          </td>
          <td
            class="enc-number"
            data-label="Itens prontos"
          >
            {{ order.itemsReady }} de {{ order.itemsTotal }}
          </td>
          <td
            class="enc-number"
            data-label="Entrega"
          >
            {{ formatCivilDate(order.deliveryDate) }}
          </td>
        </tr>
      </tbody>
    </table>
  </section>
</template>

<style scoped>
.upcoming {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
h2,
p {
  margin: 0;
}
h2 {
  font-size: 15px;
}
.muted {
  color: var(--enc-ink-secondary);
}
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
  color: var(--enc-ink);
  font-weight: 600;
}
.status {
  display: inline-flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px;
}

/* Phone: the cards of enc-cards (tokens.css); the order number is the card's title and a 48 px target. */
@media (max-width: 640px) {
  .number {
    display: flex;
    align-items: center;
    min-height: var(--enc-control-height);
  }
}
</style>
