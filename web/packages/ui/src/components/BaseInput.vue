<script setup lang="ts">
/**
 * BaseInput is the labelled text field. Its error sits right under it, with
 * an icon and linked by aria-describedby, so screen readers read it with the
 * field and it never relies on color alone.
 */
import { IconAlertCircle } from '@tabler/icons-vue'
import { computed, useId, useTemplateRef } from 'vue'

const props = withDefaults(
  defineProps<{
    /** pt-BR label, always visible. */
    label: string
    type?: 'text' | 'email' | 'password'
    /** Browser autofill hint, e.g. "username" or "current-password". */
    autocomplete?: string
    /** pt-BR message for a field the user must fix; marks the field invalid. */
    error?: string | null
    required?: boolean
  }>(),
  { type: 'text', autocomplete: 'off', error: null, required: false },
)

const model = defineModel<string>({ required: true })

const id = useId()
const errorId = `${id}-error`
const invalid = computed(() => props.error !== null && props.error !== '')
const input = useTemplateRef<HTMLInputElement>('input')

defineExpose({
  /** focus moves the keyboard to the field, e.g. to the first one a failed submit asks to fix. */
  focus: (): void => input.value?.focus(),
})
</script>

<template>
  <div class="enc-field">
    <label
      :for="id"
      class="enc-field__label"
    >{{ label }}</label>
    <input
      :id="id"
      ref="input"
      v-model="model"
      class="enc-field__input"
      :type="type"
      :autocomplete="autocomplete"
      :required="required"
      :aria-required="required ? 'true' : undefined"
      :aria-invalid="invalid ? 'true' : undefined"
      :aria-describedby="invalid ? errorId : undefined"
    >
    <p
      v-if="invalid"
      :id="errorId"
      class="enc-field__error"
    >
      <IconAlertCircle
        :size="16"
        aria-hidden="true"
      />
      {{ error }}
    </p>
  </div>
</template>

<style scoped>
.enc-field {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.enc-field__label {
  font-weight: 600;
}
.enc-field__input {
  height: var(--enc-control-height, 32px);
  padding: 0 10px;
  border: 1px solid var(--enc-secondary-border);
  border-radius: var(--enc-radius-card);
  background: var(--enc-surface);
  color: var(--enc-ink);
  font: inherit;
}
.enc-field__input[aria-invalid='true'] {
  border-color: var(--enc-status-alert-text);
}
.enc-field__error {
  display: flex;
  align-items: center;
  gap: 4px;
  margin: 0;
  color: var(--enc-status-alert-text);
}
</style>
