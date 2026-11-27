<script setup lang="ts">
/**
 * ItemsToolbar is the row above the Itens table: search, category filter,
 * "Mostrar arquivados", and the buttons that open the item form and the
 * "Categorias" and "Campos" dialogs. Typing in the search waits a moment
 * before asking for a new page; Enter asks at once.
 */
import type { Category } from '@encomendo/api'
import { BaseButton, BaseSelect } from '@encomendo/ui'
import { computed, onBeforeUnmount, ref, useId } from 'vue'

import type { ItemFilters } from '../store'

const props = defineProps<{
  filters: ItemFilters
  categories: readonly Category[]
  canWrite: boolean
}>()

const emit = defineEmits<{
  /** change carries the filters that changed; the parent applies them and reads a new page. */
  change: [patch: Partial<ItemFilters>]
  newItem: []
  categories: []
  attributes: []
}>()

const searchId = useId()
const text = ref(props.filters.q)
let timer: ReturnType<typeof setTimeout> | undefined

const categoryOptions = computed(() => [
  { value: '', label: 'Todas as categorias' },
  ...props.categories.map((category) => ({ value: category.id, label: category.name })),
])

function typed(event: Event): void {
  if (!(event.target instanceof HTMLInputElement)) return
  text.value = event.target.value
  clearTimeout(timer)
  timer = setTimeout(() => emit('change', { q: text.value }), 300)
}

function toggleArchived(event: Event): void {
  if (event.target instanceof HTMLInputElement) emit('change', { includeArchived: event.target.checked })
}

function searchNow(): void {
  clearTimeout(timer)
  emit('change', { q: text.value })
}

onBeforeUnmount(() => clearTimeout(timer))
</script>

<template>
  <div class="toolbar">
    <form
      class="search"
      role="search"
      @submit.prevent="searchNow"
    >
      <label :for="searchId">Buscar item</label>
      <input
        :id="searchId"
        type="search"
        autocomplete="off"
        :value="text"
        @input="typed"
      >
    </form>
    <BaseSelect
      :model-value="filters.categoryId"
      label="Filtrar por categoria"
      hide-label
      :options="categoryOptions"
      @update:model-value="(categoryId) => emit('change', { categoryId })"
    />
    <label class="archived enc-check">
      <input
        type="checkbox"
        :checked="filters.includeArchived"
        @change="toggleArchived"
      >
      Mostrar arquivados
    </label>
    <span class="actions">
      <BaseButton @click="emit('categories')">Categorias</BaseButton>
      <BaseButton @click="emit('attributes')">Campos</BaseButton>
      <BaseButton
        v-if="canWrite"
        variant="primary"
        @click="emit('newItem')"
      >Novo item</BaseButton>
    </span>
  </div>
</template>

<style scoped>
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
.archived {
  min-height: var(--enc-control-height, 32px);
}
.actions {
  display: flex;
  gap: 8px;
  margin-left: auto;
}

/* Phone: one column; the buttons share the last row. */
@media (max-width: 640px) {
  .toolbar {
    flex-direction: column;
    align-items: stretch;
    width: 100%;
  }
  .actions {
    flex-wrap: wrap;
    margin-left: 0;
  }
  .actions > * {
    flex: 1 1 auto;
  }
}
</style>
