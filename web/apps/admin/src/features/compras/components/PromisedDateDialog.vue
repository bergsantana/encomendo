<script setup lang="ts">
/**
 * PromisedDateDialog is "Informar prazo": "Data prometida pelo fornecedor"
 * for every open item, then "Prazo diferente para um item" with one optional
 * date per line, and "Salvar prazo". Whether a date is allowed (not before
 * today in the store's time zone) is the API's call: its refusal shows above
 * the form. A sheet on the phone, with its buttons in the sticky footer.
 */
import type { PurchaseLine } from '@encomendo/api'
import { BaseButton, BaseDialog, BaseInput } from '@encomendo/ui'
import { ref } from 'vue'

const props = defineProps<{
  /** lines are the open lines of the pedido de compra, the ones a date can still move. */
  lines: readonly PurchaseLine[]
  /** current is the date promised for the whole pedido de compra now, or null. */
  current: string | null
  error: string | null
  busy: boolean
}>()

const emit = defineEmits<{
  submit: [date: string, lines: { lineId: string; date: string }[]]
  close: []
}>()

const date = ref(props.current ?? '')
const perLine = ref<Record<string, string>>(Object.fromEntries(props.lines.map((line) => [line.id, line.promisedDate && line.promisedDate !== props.current ? line.promisedDate : ''])))
const missing = ref(false)

function submit(): void {
  missing.value = date.value === ''
  if (missing.value) return
  const different = props.lines.flatMap((line) => {
    const own = perLine.value[line.id] ?? ''
    return own === '' || own === date.value ? [] : [{ lineId: line.id, date: own }]
  })
  emit('submit', date.value, different)
}
</script>

<template>
  <BaseDialog
    title="Informar prazo"
    @close="emit('close')"
  >
    <form
      class="form"
      novalidate
      @submit.prevent="submit"
    >
      <p
        v-if="error"
        role="alert"
      >
        {{ error }}
      </p>
      <BaseInput
        v-model="date"
        type="date"
        label="Data prometida pelo fornecedor"
        :error="missing ? 'Escolha uma data a partir de hoje.' : null"
      />
      <fieldset
        v-if="lines.length > 1"
        class="lines"
      >
        <legend>Prazo diferente para um item</legend>
        <BaseInput
          v-for="line in lines"
          :key="line.id"
          :model-value="perLine[line.id] ?? ''"
          type="date"
          :label="`${line.quantity}× ${line.name} (${line.orderNumber})`"
          @update:model-value="(value) => (perLine = { ...perLine, [line.id]: value })"
        />
      </fieldset>
      <div class="buttons enc-sheet-actions">
        <BaseButton
          variant="primary"
          type="submit"
          :disabled="busy"
        >
          Salvar prazo
        </BaseButton>
        <BaseButton @click="emit('close')">
          Cancelar
        </BaseButton>
      </div>
    </form>
  </BaseDialog>
</template>

<style scoped>
.form {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
p {
  margin: 0;
}
[role='alert'] {
  color: var(--enc-status-alert-text);
  font-weight: 600;
}
.lines {
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
.buttons {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
</style>
