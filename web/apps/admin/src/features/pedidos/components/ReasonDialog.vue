<script setup lang="ts">
/**
 * ReasonDialog asks for a reason before something that is written into the
 * history: "Anular o pagamento de {valor}?" (owner) and "Cancelar o pedido
 * P-0231?". The reason is required ("Informe o motivo."). The slot holds a
 * note above the field, such as what cancelling leaves as it is. A sheet on
 * the phone, with "Voltar" in the sticky footer.
 */
import { BaseButton, BaseDialog } from '@encomendo/ui'
import { ref, useId } from 'vue'

defineProps<{
  /** title is the question, which is also the dialog's name. */
  title: string
  /** confirm is the button's words, "Anular pagamento" or "Cancelar pedido". */
  confirm: string
  error: string | null
  busy: boolean
}>()

const emit = defineEmits<{
  submit: [reason: string]
  close: []
}>()

const reason = ref('')
const missing = ref(false)
const reasonId = useId()

function submit(): void {
  missing.value = reason.value.trim() === ''
  if (!missing.value) emit('submit', reason.value.trim())
}
</script>

<template>
  <BaseDialog
    :title="title"
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
        class="error"
      >
        {{ error }}
      </p>
      <slot />
      <div class="field">
        <label :for="reasonId">Motivo</label>
        <textarea
          :id="reasonId"
          v-model="reason"
          rows="3"
          :aria-invalid="missing ? 'true' : undefined"
          :aria-describedby="missing ? `${reasonId}-error` : undefined"
        />
        <p
          v-if="missing"
          :id="`${reasonId}-error`"
          role="alert"
          class="error"
        >
          Informe o motivo.
        </p>
      </div>
      <div class="buttons enc-sheet-actions">
        <BaseButton
          variant="primary"
          type="submit"
          :disabled="busy"
        >
          {{ confirm }}
        </BaseButton>
        <BaseButton @click="emit('close')">
          Voltar
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
.field {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.field label {
  font-weight: 600;
}
textarea {
  padding: 6px 10px;
  border: 1px solid var(--enc-secondary-border);
  border-radius: var(--enc-radius-card);
  background: var(--enc-surface);
  color: var(--enc-ink);
  font: inherit;
}
.error {
  color: var(--enc-status-alert-text);
  font-weight: 600;
}
.buttons {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
</style>
