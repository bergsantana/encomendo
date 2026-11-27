<script setup lang="ts">
/**
 * KitsTable is the Kits table. "Margem" is a column only when the API sent it
 * on a row, which it does for the owner alone (ADR 0025); the screen never
 * decides that from the role. Prices, sums and days are shown as the API
 * computed them.
 */
import type { KitSummary } from '@encomendo/api'
import { computed } from 'vue'

import { formatDays, formatMargin, formatMoney } from '../../../format'
import ArchivedMark from './ArchivedMark.vue'
import PublishedSwitch from './PublishedSwitch.vue'

const props = defineProps<{
  kits: readonly KitSummary[]
  /** canWrite lets the "No portal" switch change; without it the switch only shows. */
  canWrite: boolean
  /** publishingId is the row whose "No portal" is being saved; its switch waits. */
  publishingId: string | null
}>()

const emit = defineEmits<{
  edit: [kit: KitSummary]
  publish: [kit: KitSummary, published: boolean]
}>()

const showMargin = computed(() => props.kits.some((kit) => kit.margin !== undefined))

function margin(kit: KitSummary): string {
  return kit.margin === null || kit.margin === undefined ? '—' : formatMargin(kit.margin, kit.price.currency)
}
</script>

<template>
  <table class="enc-cards">
    <caption class="enc-visually-hidden">
      Kits do catálogo
    </caption>
    <thead>
      <tr>
        <th scope="col">
          Kit
        </th>
        <th
          scope="col"
          class="number"
        >
          Itens
        </th>
        <th
          scope="col"
          class="number"
        >
          Preço do kit
        </th>
        <th
          scope="col"
          class="number"
        >
          Soma avulsa
        </th>
        <th
          v-if="showMargin"
          scope="col"
          class="number"
        >
          Margem
        </th>
        <th scope="col">
          Prazo
        </th>
        <th scope="col">
          No portal
        </th>
      </tr>
    </thead>
    <tbody>
      <tr
        v-for="kit in kits"
        :key="kit.id"
      >
        <th scope="row">
          <button
            type="button"
            class="name"
            @click="emit('edit', kit)"
          >
            {{ kit.name }}
          </button>
          <ArchivedMark v-if="kit.archived" />
        </th>
        <td
          class="number enc-number"
          data-label="Itens"
        >
          {{ kit.itemCount }}
        </td>
        <td
          class="number enc-number"
          data-label="Preço do kit"
        >
          {{ formatMoney(kit.price.cents, kit.price.currency) }}
        </td>
        <td
          class="number enc-number"
          data-label="Soma avulsa"
        >
          {{ formatMoney(kit.singleItemsTotal.cents, kit.singleItemsTotal.currency) }}
        </td>
        <td
          v-if="showMargin"
          class="number enc-number"
          data-label="Margem"
        >
          {{ margin(kit) }}
        </td>
        <td data-label="Prazo">
          {{ formatDays(kit.totalDays) }}
        </td>
        <td data-label="No portal">
          <PublishedSwitch
            :checked="kit.published"
            :label="`Mostrar ${kit.name} na página da loja`"
            :disabled="!canWrite || kit.archived"
            :busy="publishingId === kit.id"
            @toggle="(published) => emit('publish', kit, published)"
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

/* Phone: the cards of enc-cards (tokens.css), two columns of fields under the name. */
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
    min-height: var(--enc-control-height);
    text-align: left;
  }
}
</style>
