<script setup lang="ts">
/**
 * DateDialog is "Alterar data": "Nova data de entrega", "Motivo" (optional)
 * and "Salvar e avisar o cliente", which saves and opens the wa.me link the
 * API returns. Whether the date is allowed (not before today in the store's
 * time zone) is the API's call: its refusal shows above the form.
 */
import { BaseButton, BaseDialog, BaseInput } from '@encomendo/ui'
import { ref } from 'vue'

const props = defineProps<{
  /** current is the promised date now, "YYYY-MM-DD", to start from. */
  current: string
  error: string | null
  busy: boolean
}>()

const emit = defineEmits<{
  submit: [date: string, reason: string]
  close: []
}>()

const date = ref(props.current)
const reason = ref('')
const missing = ref(false)

function submit(): void {
  missing.value = date.value === ''
  if (!missing.value) emit('submit', date.value, reason.value)
}
</script>

<template>
  <BaseDialog
    title="Alterar data"
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
      <div class="field">
        <label for="new-delivery-date">Nova data de entrega</label>
        <input
          id="new-delivery-date"
          v-model="date"
          type="date"
          :aria-invalid="missing ? 'true' : undefined"
          :aria-describedby="missing ? 'new-delivery-date-error' : undefined"
        >
        <p
          v-if="missing"
          id="new-delivery-date-error"
          role="alert"
        >
          Escolha uma data a partir de hoje.
        </p>
      </div>
      <BaseInput
        v-model="reason"
        label="Motivo"
        hint="Opcional."
      />
      <div class="buttons enc-sheet-actions">
        <BaseButton
          variant="primary"
          type="submit"
          :disabled="busy"
        >
          Salvar e avisar o cliente
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
.field {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.field label {
  font-weight: 600;
}
.field input {
  height: var(--enc-control-height, 32px);
  padding: 0 10px;
  border: 1px solid var(--enc-secondary-border);
  border-radius: var(--enc-radius-card);
  background: var(--enc-surface);
  color: var(--enc-ink);
  font: inherit;
}
.buttons {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
</style>
