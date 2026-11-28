<script setup lang="ts">
/**
 * ItemsTable is "Itens": item, "Origem" (with the supplier when the API
 * sends one), "Situação" (the item's word and color), "Previsto" and the
 * actions. "Custo" shows only when the API sent a cost, which is only for
 * the owner. On the phone each item is a card. Everything shown is the API's.
 */
import { StatusBadge } from '@encomendo/ui'
import { computed } from 'vue'

import { formatCivilDate, formatMoney } from '../../../format'
import { type ItemCheck, type ItemStatus, itemStatusLabel, itemTone, type OrderItem, sourcingLabel } from '../labels'
import ItemMoveMenu from './ItemMoveMenu.vue'

const props = defineProps<{
  items: readonly OrderItem[]
  /** canMove is `order:write` on an order that is still open. */
  canMove: boolean
  busy: boolean
}>()

const emit = defineEmits<{
  move: [itemId: string, to: ItemStatus, check?: ItemCheck]
}>()

const showCost = computed(() => props.items.some((item) => item.cost !== undefined))
</script>

<template>
  <section
    class="items"
    aria-labelledby="items-heading"
  >
    <h2 id="items-heading">
      Itens
    </h2>
    <table class="enc-cards">
      <caption class="enc-visually-hidden">
        Itens do pedido
      </caption>
      <thead>
        <tr>
          <th scope="col">
            Item
          </th>
          <th scope="col">
            Origem
          </th>
          <th scope="col">
            Situação
          </th>
          <th scope="col">
            Previsto
          </th>
          <th
            v-if="showCost"
            scope="col"
            class="number"
          >
            Custo
          </th>
          <th scope="col">
            Ações
          </th>
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="item in items"
          :key="item.id"
        >
          <th scope="row">
            <span class="name">{{ item.name }}</span>
            <span
              v-if="item.quantity > 1"
              class="muted"
            > × {{ item.quantity }}</span>
            <span
              v-for="chosen in item.options"
              :key="chosen.optionId"
              class="muted option"
            >{{ chosen.name }}: {{ chosen.label }}</span>
          </th>
          <td data-label="Origem">
            {{ sourcingLabel(item.sourcingMode) }}
            <span
              v-if="item.supplier"
              class="muted supplier"
            >{{ item.supplier.name }}</span>
          </td>
          <td data-label="Situação">
            <StatusBadge
              :label="itemStatusLabel(item.status, item.sourcingMode)"
              :phase="itemTone(item.status)"
            />
          </td>
          <td
            class="enc-number"
            data-label="Previsto"
          >
            {{ item.arrivedOn !== null ? `Recebido em ${formatCivilDate(item.arrivedOn)}` : formatCivilDate(item.expectedDate) }}
          </td>
          <td
            v-if="showCost"
            class="number enc-number"
            data-label="Custo"
          >
            {{ item.cost ? formatMoney(item.cost.cents, item.cost.currency) : '—' }}
          </td>
          <td
            class="enc-cards__actions"
            data-label="Ações"
          >
            <ItemMoveMenu
              v-if="canMove"
              :item="item"
              :busy="busy"
              @move="(to, check) => emit('move', item.id, to, check)"
            />
          </td>
        </tr>
      </tbody>
    </table>
  </section>
</template>

<style scoped>
.items {
  display: flex;
  flex-direction: column;
  gap: 8px;
  width: 100%;
}
h2 {
  margin: 0;
  font-size: 15px;
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
  vertical-align: top;
}
thead th {
  color: var(--enc-ink-secondary);
}
tbody th {
  font-weight: 400;
}
.name {
  font-weight: 600;
}
.muted {
  color: var(--enc-ink-secondary);
}
.option,
.supplier {
  display: block;
}
.number {
  text-align: right;
}

/* Phone: the cards of enc-cards (tokens.css); the item's name is the card's title. */
@media (max-width: 640px) {
  .number {
    text-align: left;
  }
}
</style>
