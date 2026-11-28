<script setup lang="ts">
/**
 * OrdersToolbar is the row above the order list: the search box ("Buscar por
 * número ou cliente"), "Só com alerta" and "Novo pedido" (only for roles that
 * write orders). Typing waits a moment before asking for a new page; Enter
 * asks at once.
 */
import { BaseButton } from '@encomendo/ui'
import { onBeforeUnmount, useId } from 'vue'

defineProps<{
  canWrite: boolean
}>()

const query = defineModel<string>('query', { required: true })
const alertOnly = defineModel<boolean>('alertOnly', { required: true })

const emit = defineEmits<{
  /** search asks the parent to read the first page again. */
  search: []
  newOrder: []
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

function toggleAlert(event: Event): void {
  if (!(event.target instanceof HTMLInputElement)) return
  alertOnly.value = event.target.checked
  emit('search')
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
      <label :for="searchId">Buscar por número ou cliente</label>
      <input
        :id="searchId"
        type="search"
        autocomplete="off"
        :value="query"
        @input="typed"
      >
    </form>
    <label class="enc-check">
      <input
        type="checkbox"
        :checked="alertOnly"
        @change="toggleAlert"
      >
      Só com alerta
    </label>
    <BaseButton
      v-if="canWrite"
      variant="primary"
      class="new"
      @click="emit('newOrder')"
    >
      Novo pedido
    </BaseButton>
  </div>
</template>

<style scoped>
.toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  gap: 12px;
  width: 100%;
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
.enc-check {
  align-self: center;
}
.new {
  margin-left: auto;
}

/* Phone: the search takes the row, then "Só com alerta", then the button. */
@media (max-width: 640px) {
  .toolbar {
    flex-direction: column;
    align-items: stretch;
  }
  .enc-check {
    align-self: flex-start;
  }
  .new {
    margin-left: 0;
  }
}
</style>
