<script setup lang="ts">
/**
 * CustomerOrders is "Pedidos deste cliente" in the customer drawer: each
 * order with its number, its status (the badge takes its color from the
 * phase, its words from the order's own workflow) and "Entrega prevista".
 * The order is the one the store gave it; nothing is added up here.
 */
import type { OrderSummary } from '@encomendo/api'
import { StatusBadge } from '@encomendo/ui'
import { RouterLink } from 'vue-router'

import { formatCivilDate } from '../../../format'

defineProps<{
  orders: readonly OrderSummary[]
  loading: boolean
  error: string | null
  /** capped says the customer has more orders than were read, so the list is the nearest deliveries only. */
  capped: boolean
}>()
</script>

<template>
  <section
    class="orders"
    aria-labelledby="customer-orders"
  >
    <h2 id="customer-orders">
      Pedidos deste cliente
    </h2>
    <p
      v-if="error"
      role="alert"
    >
      {{ error }}
    </p>
    <p
      v-else-if="loading"
      class="muted"
    >
      Carregando os pedidos…
    </p>
    <p
      v-else-if="orders.length === 0"
      class="muted"
    >
      Nenhum pedido ainda.
    </p>
    <ul
      v-else
      class="list"
    >
      <li
        v-for="order in orders"
        :key="order.id"
        class="order"
      >
        <RouterLink
          class="number"
          :to="{ name: 'order', params: { orderId: order.id } }"
        >
          {{ order.number }}
        </RouterLink>
        <StatusBadge
          :label="order.status.label"
          :phase="order.status.phase"
        />
        <span class="delivery">Entrega prevista: {{ formatCivilDate(order.deliveryDate) }}</span>
      </li>
    </ul>
    <p
      v-if="capped"
      class="muted"
    >
      Mostrando os 500 pedidos com entrega mais próxima.
    </p>
  </section>
</template>

<style scoped>
.orders {
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
[role='alert'] {
  font-weight: 600;
  color: var(--enc-status-alert-text);
}
.list {
  display: flex;
  flex-direction: column;
  margin: 0;
  padding: 0;
  list-style: none;
}
.order {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px 12px;
  padding: 8px 0;
  border-bottom: 1px solid var(--enc-hairline);
}
.number {
  display: inline-flex;
  align-items: center;
  min-height: var(--enc-control-height);
  color: var(--enc-ink);
  font-family: var(--enc-font-mono);
  font-weight: 600;
}
.delivery {
  color: var(--enc-ink-secondary);
}

/* Phone: the date wraps under the number and the status. */
@media (max-width: 640px) {
  .delivery {
    flex-basis: 100%;
  }
}
</style>
