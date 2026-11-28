<script setup lang="ts">
/**
 * CustomersToolbar is the row above the customer list: the search box and
 * "Novo cliente" (only for roles that may write customers). Typing waits a
 * moment before asking for a new page; Enter asks at once.
 */
import { BaseButton } from '@encomendo/ui'
import { onBeforeUnmount, useId } from 'vue'

defineProps<{
  canWrite: boolean
}>()

const query = defineModel<string>({ required: true })

const emit = defineEmits<{
  /** search asks the parent to read the first page with the text in the box. */
  search: []
  newCustomer: []
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
  <div class="toolbar">
    <form
      class="search"
      role="search"
      @submit.prevent="searchNow"
    >
      <label :for="searchId">Buscar por nome ou WhatsApp</label>
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
      variant="primary"
      class="new"
      @click="emit('newCustomer')"
    >
      Novo cliente
    </BaseButton>
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
.new {
  margin-left: auto;
}

/* Phone: the search takes the row, the button the next one. */
@media (max-width: 640px) {
  .toolbar {
    flex-direction: column;
    align-items: stretch;
    width: 100%;
  }
  .new {
    margin-left: 0;
  }
}
</style>
