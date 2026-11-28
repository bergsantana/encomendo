<script setup lang="ts">
/**
 * CatalogPicker is the dialog of "Adicionar kit" and "Adicionar item": a
 * search box over the store's kits or items, and the matches to choose from.
 * Archived kits and items are not offered (the API lists them only on
 * request). A full-screen sheet on the phone, with its own "Fechar".
 */
import type { ItemSummary, KitSummary } from '@encomendo/api'
import { BaseButton, BaseDialog } from '@encomendo/ui'
import { onBeforeUnmount, onMounted, ref, useId } from 'vue'

import { formatMoney } from '../../../format'

const props = defineProps<{
  what: 'kits' | 'items'
  kits: readonly KitSummary[]
  items: readonly ItemSummary[]
  searching: boolean
  error: string | null
}>()

const emit = defineEmits<{
  search: [text: string]
  choose: [id: string]
  close: []
}>()

const searchId = useId()
const typed = ref('')
let timer: ReturnType<typeof setTimeout> | undefined

onMounted(() => emit('search', ''))
onBeforeUnmount(() => clearTimeout(timer))

function onInput(event: Event): void {
  if (!(event.target instanceof HTMLInputElement)) return
  typed.value = event.target.value
  clearTimeout(timer)
  timer = setTimeout(() => emit('search', typed.value), 300)
}

const title = props.what === 'kits' ? 'Adicionar kit' : 'Adicionar item'
const label = props.what === 'kits' ? 'Buscar kit' : 'Buscar item'
</script>

<template>
  <BaseDialog
    :title="title"
    @close="emit('close')"
  >
    <div class="picker">
      <p
        v-if="error"
        role="alert"
      >
        {{ error }}
      </p>
      <div class="search">
        <label :for="searchId">{{ label }}</label>
        <input
          :id="searchId"
          type="search"
          autocomplete="off"
          :value="typed"
          @input="onInput"
        >
      </div>
      <ul class="results">
        <li
          v-for="found in what === 'kits' ? kits : items"
          :key="found.id"
        >
          <button
            type="button"
            @click="emit('choose', found.id)"
          >
            <strong>{{ found.name }}</strong>
            <span class="enc-number">{{ formatMoney(found.price.cents, found.price.currency) }}</span>
          </button>
        </li>
      </ul>
      <p
        v-if="searching"
        class="muted"
      >
        Carregando…
      </p>
      <p
        v-else-if="(what === 'kits' ? kits : items).length === 0 && !error"
        class="muted"
      >
        {{ what === 'kits' ? 'Nenhum kit encontrado.' : 'Nenhum item encontrado.' }}
      </p>
      <div class="buttons enc-sheet-actions">
        <BaseButton @click="emit('close')">
          Fechar
        </BaseButton>
      </div>
    </div>
  </BaseDialog>
</template>

<style scoped>
.picker {
  display: flex;
  flex-direction: column;
  gap: 12px;
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
.search {
  display: flex;
  flex-direction: column;
  gap: 4px;
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
.results {
  display: flex;
  flex-direction: column;
  margin: 0;
  padding: 0;
  list-style: none;
}
.results button {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 4px 12px;
  width: 100%;
  min-height: var(--enc-control-height);
  padding: 4px 10px;
  border: 0;
  border-bottom: 1px solid var(--enc-hairline);
  background: var(--enc-surface);
  color: var(--enc-ink);
  font: inherit;
  text-align: left;
  cursor: pointer;
}
.buttons {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
</style>
