<script setup lang="ts">
/**
 * CustomerPicker is "Cliente" on the order form: "Buscar cliente" (a name or
 * a WhatsApp number) with the matches to choose from, or "Novo cliente" with
 * "Nome" and "WhatsApp". Once chosen it shows the customer and "Trocar
 * cliente". On a change the customer is fixed and only shown.
 */
import type { Customer, CustomerSummary } from '@encomendo/api'
import { BaseButton, BaseInput } from '@encomendo/ui'
import { onBeforeUnmount, reactive, ref, useId } from 'vue'

import { formatWhatsapp } from '../../../phone'

defineProps<{
  customer: Customer | null
  /** locked is true on "Alterar pedido": the order keeps its customer. */
  locked: boolean
  results: readonly CustomerSummary[]
  searching: boolean
  /** error is the message above the picker; `fieldErrors` the ones next to Nome and WhatsApp. */
  error: string | null
  fieldErrors: Readonly<Record<string, string>>
  creating: boolean
  /** orderError is "Informe o cliente do pedido." */
  orderError: string | null
}>()

const emit = defineEmits<{
  search: [text: string]
  choose: [customerId: string]
  clear: []
  create: [name: string, phone: string]
}>()

const searchId = useId()
const typed = ref('')
const adding = ref(false)
const fresh = reactive({ name: '', phone: '' })
let timer: ReturnType<typeof setTimeout> | undefined

function onSearch(event: Event): void {
  if (!(event.target instanceof HTMLInputElement)) return
  typed.value = event.target.value
  clearTimeout(timer)
  timer = setTimeout(() => emit('search', typed.value), 300)
}

function searchNow(): void {
  clearTimeout(timer)
  emit('search', typed.value)
}

onBeforeUnmount(() => clearTimeout(timer))
</script>

<template>
  <section
    class="picker"
    aria-labelledby="customer-heading"
  >
    <h2 id="customer-heading">
      Cliente
    </h2>

    <div
      v-if="customer !== null"
      class="chosen"
    >
      <p>
        <strong>{{ customer.name }}</strong>
        <span class="muted">{{ formatWhatsapp(customer.phone) }}</span>
      </p>
      <BaseButton
        v-if="!locked"
        @click="emit('clear')"
      >
        Trocar cliente
      </BaseButton>
    </div>

    <template v-else>
      <p
        v-if="error"
        role="alert"
      >
        {{ error }}
      </p>
      <p
        v-if="orderError"
        class="error"
      >
        {{ orderError }}
      </p>

      <form
        v-if="!adding"
        class="search"
        role="search"
        @submit.prevent="searchNow"
      >
        <label :for="searchId">Buscar cliente</label>
        <input
          :id="searchId"
          type="search"
          autocomplete="off"
          :value="typed"
          @input="onSearch"
        >
      </form>
      <ul
        v-if="!adding && results.length > 0"
        class="results"
      >
        <li
          v-for="found in results"
          :key="found.id"
        >
          <button
            type="button"
            @click="emit('choose', found.id)"
          >
            <strong>{{ found.name }}</strong>
            <span class="muted">{{ formatWhatsapp(found.phone) }}</span>
          </button>
        </li>
      </ul>
      <p
        v-else-if="!adding && typed.trim() !== '' && !searching"
        class="muted"
      >
        Nenhum cliente encontrado.
      </p>

      <form
        v-if="adding"
        class="fresh"
        novalidate
        @submit.prevent="emit('create', fresh.name, fresh.phone)"
      >
        <BaseInput
          v-model="fresh.name"
          label="Nome"
          :error="fieldErrors.name ?? null"
        />
        <BaseInput
          v-model="fresh.phone"
          label="WhatsApp"
          hint="Com DDD, como (92) 99123-4567."
          inputmode="tel"
          :error="fieldErrors.phone ?? null"
        />
        <div class="buttons">
          <BaseButton
            type="submit"
            :disabled="creating"
          >
            Criar cliente
          </BaseButton>
          <BaseButton @click="adding = false">
            Cancelar
          </BaseButton>
        </div>
      </form>
      <div v-else>
        <BaseButton @click="adding = true">
          Novo cliente
        </BaseButton>
      </div>
    </template>
  </section>
</template>

<style scoped>
.picker {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
h2,
p {
  margin: 0;
}
h2 {
  font-size: 15px;
}
.chosen {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 16px;
}
.muted {
  margin-left: 8px;
  color: var(--enc-ink-secondary);
}
.error,
[role='alert'] {
  color: var(--enc-status-alert-text);
  font-weight: 600;
}
.search {
  display: flex;
  flex-direction: column;
  gap: 4px;
  max-width: 360px;
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
  max-width: 360px;
  margin: 0;
  padding: 0;
  list-style: none;
  border: 1px solid var(--enc-hairline);
  border-radius: var(--enc-radius-card);
}
.results button {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
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
.fresh {
  display: flex;
  flex-direction: column;
  gap: 12px;
  max-width: 360px;
}
.buttons {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

/* Phone: the search and the results take the whole row. */
@media (max-width: 640px) {
  .search,
  .results,
  .fresh {
    max-width: none;
  }
}
</style>
