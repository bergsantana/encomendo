<script setup lang="ts">
/**
 * ToReceivePanel is "A receber" and "A devolver": the balances still due on
 * active orders, whatever the period (the server's total, how many orders and
 * the list by delivery date, each with its own balance), and the orders with
 * "Valor a devolver", when there are any. Nothing is added up here.
 */
import type { FinanceSummary } from '@encomendo/api'
import { StatusBadge } from '@encomendo/ui'
import { RouterLink } from 'vue-router'

import { formatCivilDate, formatMoney } from '../../../format'

defineProps<{
  toReceive: FinanceSummary['toReceive']
  toRefund: FinanceSummary['toRefund']
}>()
</script>

<template>
  <section
    class="to-receive"
    aria-labelledby="to-receive-title"
  >
    <h2 id="to-receive-title">
      A receber
    </h2>
    <p class="total enc-number">
      {{ formatMoney(toReceive.total.cents, toReceive.total.currency) }}
    </p>
    <p class="muted">
      {{ toReceive.orders === 1 ? '1 pedido com saldo' : `${toReceive.orders} pedidos com saldo` }}
    </p>
    <table
      v-if="toReceive.list.length > 0"
      class="enc-cards"
    >
      <caption class="enc-visually-hidden">
        Pedidos com saldo a receber
      </caption>
      <thead>
        <tr>
          <th scope="col">
            Pedido
          </th>
          <th scope="col">
            Cliente
          </th>
          <th
            scope="col"
            class="amount"
          >
            Saldo
          </th>
          <th scope="col">
            Entrega prevista
          </th>
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="order in toReceive.list"
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
          <td
            class="amount enc-number"
            data-label="Saldo"
          >
            {{ formatMoney(order.balanceDue.cents, order.balanceDue.currency) }}
          </td>
          <td
            class="enc-number"
            data-label="Entrega prevista"
          >
            {{ formatCivilDate(order.deliveryDate) }}
          </td>
        </tr>
      </tbody>
    </table>
  </section>

  <section
    v-if="toRefund.length > 0"
    class="to-refund"
    aria-labelledby="to-refund-title"
  >
    <h2 id="to-refund-title">
      A devolver
    </h2>
    <table class="enc-cards">
      <caption class="enc-visually-hidden">
        Pedidos com valor a devolver
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
            class="amount"
          >
            Valor a devolver
          </th>
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="entry in toRefund"
          :key="entry.order.id"
        >
          <th scope="row">
            <RouterLink
              class="number enc-code"
              :to="{ name: 'order', params: { orderId: entry.order.id } }"
            >
              {{ entry.order.number }}
            </RouterLink>
          </th>
          <td data-label="Cliente">
            {{ entry.order.customer.name }}
          </td>
          <td data-label="Status">
            <StatusBadge
              :label="entry.order.status.label"
              :phase="entry.order.status.phase"
            />
          </td>
          <td
            class="amount enc-number"
            data-label="Valor a devolver"
          >
            {{ formatMoney(entry.amount.cents, entry.amount.currency) }}
          </td>
        </tr>
      </tbody>
    </table>
  </section>
</template>

<style scoped>
section {
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
.total {
  font-size: 28px;
  line-height: 32px;
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
.amount {
  text-align: right;
}
.number {
  color: var(--enc-ink);
  font-weight: 600;
}

/* Phone: the cards of enc-cards (tokens.css); the order number is the card's title and a 48 px target. */
@media (max-width: 640px) {
  .amount {
    text-align: left;
  }
  .number {
    display: flex;
    align-items: center;
    min-height: var(--enc-control-height);
  }
}
</style>
