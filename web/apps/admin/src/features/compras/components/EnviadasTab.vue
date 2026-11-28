<script setup lang="ts">
/**
 * EnviadasTab is Compras, "Enviadas": the store's pedidos de compra, newest
 * first, "Abertos" (sent or in transit) or "Todos". "Prazo do fornecedor" is
 * "Sem prazo" until the supplier's date is recorded, and "Recebidos" counts
 * the lines received or found defective. On the phone each is a card.
 */
import type { SupplierOrderSummary } from '@encomendo/api'
import { BaseButton, StatusBadge } from '@encomendo/ui'
import { useId } from 'vue'
import { RouterLink } from 'vue-router'

import { formatCivilDate, formatDate } from '../../../format'
import { orderStateLabel, orderStateTone } from '../labels'

defineProps<{
  orders: readonly SupplierOrderSummary[]
  filter: 'open' | 'all'
  loaded: boolean
  loading: boolean
  error: string | null
  hasMore: boolean
  /** timeZone is the store's, so "Enviado em" shows the day the store lived it. */
  timeZone?: string
}>()

const emit = defineEmits<{
  filter: [filter: 'open' | 'all']
  loadMore: []
}>()

const group = useId()
const choices = [
  { value: 'open', label: 'Abertos' },
  { value: 'all', label: 'Todos' },
] as const
</script>

<template>
  <div class="tab">
    <fieldset class="filter">
      <legend class="enc-visually-hidden">
        Pedidos de compra
      </legend>
      <label
        v-for="choice in choices"
        :key="choice.value"
        class="enc-check"
      >
        <input
          type="radio"
          :name="group"
          :value="choice.value"
          :checked="filter === choice.value"
          @change="emit('filter', choice.value)"
        >
        {{ choice.label }}
      </label>
    </fieldset>

    <p
      v-if="error"
      role="alert"
    >
      {{ error }}
    </p>

    <table
      v-if="orders.length > 0"
      class="enc-cards"
    >
      <caption class="enc-visually-hidden">
        Pedidos de compra enviados
      </caption>
      <thead>
        <tr>
          <th scope="col">
            Pedido de compra
          </th>
          <th scope="col">
            Fornecedor
          </th>
          <th scope="col">
            Enviado em
          </th>
          <th scope="col">
            Prazo do fornecedor
          </th>
          <th scope="col">
            Recebidos
          </th>
          <th scope="col">
            Situação
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
              class="number-link enc-code"
              :to="{ name: 'supplier-order', params: { supplierOrderId: order.id } }"
            >
              {{ order.number }}
            </RouterLink>
          </th>
          <td data-label="Fornecedor">
            {{ order.supplierName }}
          </td>
          <td data-label="Enviado em">
            {{ formatDate(order.sentAt, timeZone) }}
          </td>
          <td data-label="Prazo do fornecedor">
            {{ order.promisedDate ? formatCivilDate(order.promisedDate) : 'Sem prazo' }}
          </td>
          <td
            data-label="Recebidos"
            class="enc-number"
          >
            {{ order.received }} de {{ order.total }}
          </td>
          <td data-label="Situação">
            <StatusBadge
              :label="orderStateLabel(order.state)"
              :phase="orderStateTone(order.state)"
            />
          </td>
        </tr>
      </tbody>
    </table>
    <p
      v-else-if="!loaded && !error"
      class="muted"
    >
      Carregando as compras…
    </p>
    <p
      v-else-if="loaded"
      class="muted"
    >
      Nenhum pedido de compra ainda.
    </p>

    <BaseButton
      v-if="hasMore"
      :disabled="loading"
      @click="emit('loadMore')"
    >
      Carregar mais
    </BaseButton>
  </div>
</template>

<style scoped>
.tab {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 16px;
  width: 100%;
}
p {
  margin: 0;
}
.muted {
  color: var(--enc-ink-secondary);
}
[role='alert'] {
  color: var(--enc-status-alert-text);
  font-weight: 600;
}
.filter {
  display: flex;
  gap: 16px;
  margin: 0;
  padding: 0;
  border: 0;
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
.number-link {
  color: var(--enc-ink);
  font-weight: 600;
}

/* Phone: the cards of enc-cards (tokens.css); the C-number is the card's title and a 48 px target. */
@media (max-width: 640px) {
  .number-link {
    display: flex;
    align-items: center;
    min-height: var(--enc-control-height);
  }
}
</style>
