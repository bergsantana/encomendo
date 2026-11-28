<script setup lang="ts">
/**
 * PeriodPicker is the period of Financeiro: "Este mês", "Mês passado",
 * "Últimos 30 dias" or "Escolher período", which shows "De" and "Até". It
 * only reports what the owner chose; the store turns a preset into days and
 * the API checks the period.
 */
import { BaseInput, BaseSelect } from '@encomendo/ui'
import { computed } from 'vue'

import { isPreset, type PeriodPreset, presetLabels } from '../period'

const props = defineProps<{
  preset: PeriodPreset
  /** from and to are the days in effect, "YYYY-MM-DD"; they fill the two fields of "Escolher período". */
  from: string
  to: string
  /**
   * error is the pt-BR message of the last read. With "Escolher período" it sits under "Até",
   * linked to the field; otherwise it sits under the picker.
   */
  error?: string | null
}>()

const emit = defineEmits<{
  choose: [preset: PeriodPreset]
  /** days is the owner's change of "De" or "Até". */
  days: [from: string, to: string]
}>()

const choice = computed({
  get: () => props.preset,
  set: (value: string) => {
    if (isPreset(value)) emit('choose', value)
  },
})

// A day is reported when the field's value is complete: the date input only changes then.
const fromDay = computed({
  get: () => props.from,
  set: (value: string) => emit('days', value, props.to),
})
const toDay = computed({
  get: () => props.to,
  set: (value: string) => emit('days', props.from, value),
})
</script>

<template>
  <div class="picker">
    <BaseSelect
      v-model="choice"
      label="Período"
      :options="presetLabels"
    />
    <template v-if="preset === 'custom'">
      <BaseInput
        v-model="fromDay"
        type="date"
        label="De"
      />
      <BaseInput
        v-model="toDay"
        type="date"
        label="Até"
        :error="error"
      />
    </template>
    <p
      v-else-if="error"
      role="alert"
      class="error"
    >
      {{ error }}
    </p>
  </div>
</template>

<style scoped>
.picker {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  gap: 12px;
}
.error {
  flex-basis: 100%;
  margin: 0;
  font-weight: 600;
  color: var(--enc-status-alert-text);
}

/* Phone: every field takes the full width. */
@media (max-width: 640px) {
  .picker > * {
    flex: 1 1 100%;
  }
}
</style>
