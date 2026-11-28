<script setup lang="ts">
/**
 * CustomerOrders is "Pedidos deste cliente" in the customer drawer: each
 * order with its number, its status (the badge takes its color from the
 * phase, its words from the order's own workflow) and "Entrega prevista".
 * The order is the one the store gave it; nothing is added up here.
 */
import type { OrderSummary } from '@encomendo/api'
import { StatusBadge } from '@encomendo/ui'

import { formatCivilDate } from '../../../format'

defineProps<{
  orders: readonly OrderSummary[]
  loading: boolean
  error: string | null
}>()
</script>

<template>
  <section
    class="orders"
    aria-labelledby="customer-orders"
  >
    <h3 id="customer-orders">
      Pedidos deste cliente
    </h3>
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
        <span class="number">{{ order.number }}</span>
        <StatusBadge
          :label="order.status.label"
          :phase="order.status.phase"
        />
        <span class="delivery">Entrega prevista: {{ formatCivilDate(order.deliveryDate) }}</span>
      </li>
    </ul>
  </section>
</template>

<style scoped>
.orders {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
h3,
p {
  margin: 0;
}
h3 {
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
