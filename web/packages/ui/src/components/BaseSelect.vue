<script setup lang="ts">
/**
 * BaseSelect is the labelled choice from a short list, such as a member's
 * role. Like BaseInput, its error sits right under it with an icon, linked
 * by aria-describedby, and extra attributes go to the select behind its own
 * id and links.
 */
import { IconAlertCircle } from '@tabler/icons-vue'
import { computed, useId } from 'vue'

/** SelectOption is one choice: the value the app keeps and the pt-BR label shown. */
export interface SelectOption {
  value: string
  label: string
}

defineOptions({ inheritAttrs: false })

const props = withDefaults(
  defineProps<{
    /** pt-BR label, always visible. Hide it visually with `hideLabel` where a table header already says it. */
    label: string
    options: readonly SelectOption[]
    /** pt-BR message for a choice the user must fix; marks the field invalid. */
    error?: string | null
    disabled?: boolean
    /** hideLabel keeps the label for screen readers only, e.g. inside a table row. */
    hideLabel?: boolean
  }>(),
  { error: null, disabled: false, hideLabel: false },
)

const model = defineModel<string>({ required: true })

const id = useId()
const errorId = `${id}-error`
const invalid = computed(() => props.error !== null && props.error !== '')
</script>

<template>
  <div class="enc-field">
    <label
      :for="id"
      class="enc-field__label"
      :class="{ 'enc-visually-hidden': hideLabel }"
    >{{ label }}</label>
    <select
      v-bind="$attrs"
      :id="id"
      v-model="model"
      class="enc-field__select"
      :disabled="disabled"
      :aria-invalid="invalid ? 'true' : undefined"
      :aria-describedby="invalid ? errorId : undefined"
    >
      <option
        v-for="option in options"
        :key="option.value"
        :value="option.value"
      >
        {{ option.label }}
      </option>
    </select>
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
.enc-field__select {
  height: var(--enc-control-height, 32px);
  padding: 0 8px;
  border: 1px solid var(--enc-secondary-border);
  border-radius: var(--enc-radius-card);
  background: var(--enc-surface);
  color: var(--enc-ink);
  font: inherit;
}
.enc-field__select[aria-invalid='true'] {
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
