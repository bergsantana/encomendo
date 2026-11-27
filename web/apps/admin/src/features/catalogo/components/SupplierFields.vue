<script setup lang="ts">
/**
 * SupplierFields is one supplier slot of an item, main or backup: the choice
 * of supplier (with "Novo fornecedor" inline), its lead time and, for the
 * owner only, its cost. A cost belongs to the item and that supplier: picking
 * a supplier the item already had brings back its own cost, any other starts
 * with none (ADR 0025).
 */
import type { Supplier } from '@encomendo/api'
import { BaseButton, BaseInput, BaseSelect } from '@encomendo/ui'
import { computed, ref, watch } from 'vue'

import type { SupplierDraft } from '../itemForm'

const props = defineProps<{
  /** legend names the group: "Fornecedor" for the main one, "Fornecedor reserva" for the backup. */
  legend: string
  /** supplierLabel is the label of the choice: "Fornecedor principal" or "Fornecedor reserva". */
  supplierLabel: string
  suppliers: readonly Supplier[]
  /** showCost is true for the owner only. */
  showCost: boolean
  /** storedCosts are the item's own cost texts by supplier, as read. */
  storedCosts: Readonly<Record<string, string>>
  /** errors are the messages next to the fields: `supplier`, `leadDays` and `cost`. */
  errors: Readonly<Record<string, string>>
  /** adding shows the "Novo fornecedor" form in this slot; the drawer decides, so only one slot has it. */
  adding: boolean
  creating: boolean
  createError: string | null
  createFieldErrors: Readonly<Record<string, string>>
}>()

const draft = defineModel<SupplierDraft>({ required: true })

const emit = defineEmits<{
  startNew: []
  createSupplier: [name: string, whatsapp: string]
  cancelNew: []
}>()

const NEW = 'new'
const name = ref('')
const whatsapp = ref('')

const options = computed(() => [
  { value: '', label: 'Escolha o fornecedor' },
  ...props.suppliers.map((supplier) => ({ value: supplier.id, label: supplier.name })),
  { value: NEW, label: 'Novo fornecedor' },
])

const choice = computed({
  get: () => (props.adding ? NEW : draft.value.supplierId),
  set: (value: string) => {
    if (value === NEW) {
      emit('startNew')
      return
    }
    if (props.adding) emit('cancelNew')
    draft.value = { ...draft.value, supplierId: value, cost: props.storedCosts[value] ?? '' }
  },
})

function create(): void {
  emit('createSupplier', name.value, whatsapp.value)
}

function cancel(): void {
  emit('cancelNew')
}

// A closed form starts empty the next time.
watch(
  () => props.adding,
  (open) => {
    if (open) return
    name.value = ''
    whatsapp.value = ''
  },
)
</script>

<template>
  <fieldset class="group">
    <legend>{{ legend }}</legend>
    <BaseSelect
      v-model="choice"
      :label="supplierLabel"
      :options="options"
      :error="errors.supplier ?? null"
    />
    <div
      v-if="adding"
      class="new"
      role="group"
      aria-label="Novo fornecedor"
    >
      <p
        v-if="createError"
        role="alert"
      >
        {{ createError }}
      </p>
      <BaseInput
        v-model="name"
        label="Nome do fornecedor"
        :error="createFieldErrors.name ?? null"
      />
      <BaseInput
        v-model="whatsapp"
        label="WhatsApp do fornecedor"
        hint="Com DDD, como (11) 98765-4321."
        inputmode="tel"
        autocomplete="tel"
        :error="createFieldErrors.whatsappPhone ?? null"
      />
      <div class="row">
        <BaseButton
          :disabled="creating"
          @click="create"
        >
          Adicionar fornecedor
        </BaseButton>
        <BaseButton @click="cancel">
          Cancelar
        </BaseButton>
      </div>
    </div>
    <BaseInput
      :model-value="draft.leadDays"
      label="Prazo do fornecedor (dias)"
      inputmode="numeric"
      :error="errors.leadDays ?? null"
      @update:model-value="(leadDays) => (draft = { ...draft, leadDays })"
    />
    <BaseInput
      v-if="showCost"
      :model-value="draft.cost"
      label="Custo"
      inputmode="decimal"
      :error="errors.cost ?? null"
      @update:model-value="(cost) => (draft = { ...draft, cost })"
    />
    <slot />
  </fieldset>
</template>

<style scoped>
.group {
  display: flex;
  flex-direction: column;
  gap: 12px;
  margin: 0;
  padding: 12px;
  border: 1px solid var(--enc-hairline);
  border-radius: var(--enc-radius-card);
}
legend {
  padding: 0 4px;
  font-weight: 600;
}
.new {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 12px;
  background: var(--enc-ground);
  border-radius: var(--enc-radius-card);
}
.row {
  display: flex;
  gap: 8px;
}
p {
  margin: 0;
  font-weight: 600;
}
</style>
