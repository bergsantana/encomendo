<script setup lang="ts">
/**
 * CustomerPicker is "Cliente" on the order form: "Buscar cliente" (a name or
 * a WhatsApp number) with the matches to choose from, or "Novo cliente" with
 * "Nome" and "WhatsApp" (made with the order, so "Criar cliente" only takes
 * the two fields for it). Once chosen it shows the customer and "Trocar
 * cliente". On a change the customer is fixed and only shown.
 */
import type { CustomerSummary } from '@encomendo/api'
import { BaseButton, BaseInput } from '@encomendo/ui'
import { onBeforeUnmount, reactive, ref, useId } from 'vue'

import { formatWhatsapp } from '../../../phone'

defineProps<{
  /** chosen is the customer on the order (found, or typed under "Novo cliente"), or null. */
  chosen: { name: string; phone: string } | null
  /** locked is true on "Alterar pedido": the order keeps its customer. */
  locked: boolean
  results: readonly CustomerSummary[]
  searching: boolean
  /** error is the message above the picker; `fieldErrors` the ones next to Nome and WhatsApp. */
  error: string | null
  fieldErrors: Readonly<Record<string, string>>
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

/** startOver takes the customer off and leaves the form as new: the search box, with no Nome or WhatsApp left over. */
function startOver(): void {
  adding.value = false
  fresh.name = ''
  fresh.phone = ''
  typed.value = ''
  emit('clear')
}

function create(): void {
  emit('create', fresh.name, fresh.phone)
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
      v-if="chosen !== null"
      class="chosen"
    >
      <p>
        <strong>{{ chosen.name }}</strong>
        <span class="muted">{{ formatWhatsapp(chosen.phone) }}</span>
      </p>
      <BaseButton
        v-if="!locked"
        @click="startOver"
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
        :id="`${searchId}-error`"
        role="alert"
        class="error"
      >
        {{ orderError }}
      </p>

      <!-- Plain blocks, not forms: this picker sits inside the order's own form, and a submit here would save the order. -->
      <div
        v-if="!adding"
        class="search"
        role="search"
      >
        <label :for="searchId">Buscar cliente</label>
        <input
          :id="searchId"
          type="search"
          autocomplete="off"
          :value="typed"
          :aria-invalid="orderError ? 'true' : undefined"
          :aria-describedby="orderError ? `${searchId}-error` : undefined"
          @input="onSearch"
          @keydown.enter.prevent="searchNow"
        >
      </div>
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

      <div
        v-if="adding"
        class="fresh"
      >
        <BaseInput
          v-model="fresh.name"
          label="Nome"
          :error="fieldErrors.name ?? null"
          @keydown.enter.prevent="create"
        />
        <BaseInput
          v-model="fresh.phone"
          label="WhatsApp"
          hint="Com DDD, como (92) 99123-4567."
          inputmode="tel"
          :error="fieldErrors.phone ?? null"
          @keydown.enter.prevent="create"
        />
        <div class="buttons">
          <BaseButton @click="create">
            Criar cliente
          </BaseButton>
          <BaseButton @click="adding = false">
            Cancelar
          </BaseButton>
        </div>
      </div>
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
