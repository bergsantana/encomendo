<script setup lang="ts">
/**
 * ItemsTable is the Itens table. "Custo" and "Margem" are columns only when
 * the API sent them on a row, which it does for the owner alone (ADR 0025);
 * the screen never decides that from the role. Prices, margins and days are
 * shown as the API computed them.
 */
import type { Category, ItemSummary } from '@encomendo/api'
import { IconArchive } from '@encomendo/ui'
import { computed } from 'vue'

import { formatDays, formatMargin, formatMoney } from '../../../format'
import { sourcingLabel } from '../labels'
import PublishedSwitch from './PublishedSwitch.vue'

const props = defineProps<{
  items: readonly ItemSummary[]
  categories: readonly Category[]
  /** canWrite lets the "No portal" switch change; without it the switch only shows. */
  canWrite: boolean
  /** publishingId is the row whose "No portal" is being saved; its switch waits. */
  publishingId: string | null
}>()

const emit = defineEmits<{
  edit: [item: ItemSummary]
  publish: [item: ItemSummary, published: boolean]
}>()

const showCosts = computed(() => props.items.some((item) => item.cost !== undefined || item.margin !== undefined))
const categoryNames = computed(() => new Map(props.categories.map((category) => [category.id, category.name])))

function categoryName(item: ItemSummary): string {
  return item.categoryId === null ? 'Sem categoria' : (categoryNames.value.get(item.categoryId) ?? 'Sem categoria')
}

function cost(item: ItemSummary): string {
  return item.cost === null || item.cost === undefined ? '—' : formatMoney(item.cost.cents, item.cost.currency)
}

function margin(item: ItemSummary): string {
  return item.margin === null || item.margin === undefined ? '—' : formatMargin(item.margin, item.price.currency)
}
</script>

<template>
  <table>
    <caption class="enc-visually-hidden">
      Itens do catálogo
    </caption>
    <thead>
      <tr>
        <th scope="col">
          Item
        </th>
        <th scope="col">
          Categoria
        </th>
        <th scope="col">
          Origem
        </th>
        <th scope="col">
          Fornecedor
        </th>
        <th scope="col">
          Prazo
        </th>
        <th
          v-if="showCosts"
          scope="col"
          class="number"
        >
          Custo
        </th>
        <th
          scope="col"
          class="number"
        >
          Preço
        </th>
        <th
          v-if="showCosts"
          scope="col"
          class="number"
        >
          Margem
        </th>
        <th scope="col">
          No portal
        </th>
      </tr>
    </thead>
    <tbody>
      <tr
        v-for="item in items"
        :key="item.id"
      >
        <th scope="row">
          <button
            type="button"
            class="name"
            @click="emit('edit', item)"
          >
            {{ item.name }}
          </button>
          <span
            v-if="item.archived"
            class="archived"
          >
            <IconArchive
              :size="14"
              aria-hidden="true"
            />
            Arquivado
          </span>
        </th>
        <td>{{ categoryName(item) }}</td>
        <td>{{ sourcingLabel(item.sourcingMode) }}</td>
        <td>{{ item.mainSupplierName ?? '—' }}</td>
        <td>{{ formatDays(item.readyDays) }}</td>
        <td
          v-if="showCosts"
          class="number enc-number"
        >
          {{ cost(item) }}
        </td>
        <td class="number enc-number">
          {{ formatMoney(item.price.cents, item.price.currency) }}
        </td>
        <td
          v-if="showCosts"
          class="number enc-number"
        >
          {{ margin(item) }}
        </td>
        <td>
          <PublishedSwitch
            :checked="item.published"
            :label="`Mostrar ${item.name} na página da loja`"
            :disabled="!canWrite || item.archived"
            :busy="publishingId === item.id"
            @toggle="(published) => emit('publish', item, published)"
          />
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
  padding: 0;
  border: 0;
  background: none;
  color: var(--enc-ink);
  font: inherit;
  font-weight: 600;
  text-decoration: underline;
  cursor: pointer;
}
.archived {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  margin-left: 8px;
  padding: 0 6px;
  border-radius: var(--enc-radius-badge);
  background: var(--enc-status-neutral-fill);
  color: var(--enc-status-neutral-text);
}
</style>
