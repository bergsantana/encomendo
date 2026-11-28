<script setup lang="ts">
/**
 * FornecedoresTab is Compras, "Fornecedores": the store's suppliers, "Buscar
 * fornecedor" and "Novo fornecedor". A supplier's name opens its drawer. On
 * the phone each is a card. Reading is for anyone who reads the catalog;
 * "Novo fornecedor" needs `catalog:write`.
 */
import type { Supplier } from '@encomendo/api'
import { BaseButton } from '@encomendo/ui'
import { onBeforeUnmount, useId } from 'vue'

import { formatWhatsapp } from '../../../phone'

defineProps<{
  suppliers: readonly Supplier[]
  loaded: boolean
  loading: boolean
  error: string | null
  hasMore: boolean
  notice: string | null
  canWrite: boolean
}>()

const query = defineModel<string>({ required: true })

const emit = defineEmits<{
  search: []
  open: [supplier: Supplier | null]
  loadMore: []
}>()

const searchId = useId()
let timer: ReturnType<typeof setTimeout> | undefined

function typed(event: Event): void {
  if (!(event.target instanceof HTMLInputElement)) return
  query.value = event.target.value
  clearTimeout(timer)
  timer = setTimeout(() => emit('search'), 300)
}

function searchNow(): void {
  clearTimeout(timer)
  emit('search')
}

onBeforeUnmount(() => clearTimeout(timer))
</script>

<template>
  <div class="tab">
    <p
      v-if="notice"
      role="status"
      class="strong"
    >
      {{ notice }}
    </p>
    <p
      v-if="error"
      role="alert"
      class="strong error"
    >
      {{ error }}
    </p>

    <div class="toolbar">
      <form
        class="search"
        role="search"
        @submit.prevent="searchNow"
      >
        <label :for="searchId">Buscar fornecedor</label>
        <input
          :id="searchId"
          type="search"
          autocomplete="off"
          :value="query"
          @input="typed"
        >
      </form>
      <BaseButton
        v-if="canWrite"
        @click="emit('open', null)"
      >
        Novo fornecedor
      </BaseButton>
    </div>

    <table
      v-if="suppliers.length > 0"
      class="enc-cards"
    >
      <caption class="enc-visually-hidden">
        Fornecedores da loja
      </caption>
      <thead>
        <tr>
          <th scope="col">
            Fornecedor
          </th>
          <th scope="col">
            WhatsApp
          </th>
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="supplier in suppliers"
          :key="supplier.id"
        >
          <th scope="row">
            <button
              type="button"
              class="name"
              @click="emit('open', supplier)"
            >
              {{ supplier.name }}
            </button>
          </th>
          <td data-label="WhatsApp">
            {{ formatWhatsapp(supplier.whatsappPhone) }}
          </td>
        </tr>
      </tbody>
    </table>
    <p
      v-else-if="!loaded && !error"
      class="muted"
    >
      Carregando os fornecedores…
    </p>
    <p
      v-else-if="loaded && query.trim() === ''"
      class="muted"
    >
      Nenhum fornecedor ainda. Você também pode criar um fornecedor no cadastro do item.
    </p>
    <p
      v-else-if="loaded"
      class="muted"
    >
      Nenhum fornecedor encontrado.
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
.strong {
  font-weight: 600;
}
.error {
  color: var(--enc-status-alert-text);
}
.toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  gap: 12px;
}
.search {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: min(320px, 100%);
}
.search label {
  font-weight: 600;
}
.search input {
  height: var(--enc-control-height, 32px);
  padding: 0 10px;
  border: 1px solid var(--enc-secondary-border);
  border-radius: var(--enc-radius-card);
  background: var(--enc-surface);
  color: var(--enc-ink);
  font: inherit;
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

/* Phone: the cards of enc-cards (tokens.css); the name is the card's title and a 48 px target; the toolbar stacks. */
@media (max-width: 640px) {
  .toolbar {
    flex-direction: column;
    align-items: stretch;
    width: 100%;
  }
  .name {
    display: flex;
    align-items: center;
    min-height: var(--enc-control-height);
  }
}
</style>
