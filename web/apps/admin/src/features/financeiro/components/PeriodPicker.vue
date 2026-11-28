<script setup lang="ts">
/**
 * PeriodPicker is the period of Financeiro: "Este mês", "Mês passado",
 * "Últimos 30 dias" or "Escolher período", which shows "De" and "Até". It
 * only reports what the owner chose; the store turns a preset into days and
 * the API checks the period.
 */
import { BaseSelect } from '@encomendo/ui'
import { computed, useId } from 'vue'

import { isPreset, type PeriodPreset, presetLabels } from '../period'

const props = defineProps<{
  preset: PeriodPreset
  /** from and to are the days in effect, "YYYY-MM-DD"; they fill the two fields of "Escolher período". */
  from: string
  to: string
  /** error is the pt-BR message under the picker, linked to its fields. */
  error?: string | null
}>()

const emit = defineEmits<{
  choose: [preset: PeriodPreset]
  /** days is the owner's change of "De" or "Até". */
  days: [from: string, to: string]
}>()

const fromId = useId()
const toId = useId()
const errorId = useId()

const choice = computed({
  get: () => props.preset,
  set: (value: string) => {
    if (isPreset(value)) emit('choose', value)
  },
})

function typed(event: Event, which: 'from' | 'to'): void {
  if (!(event.target instanceof HTMLInputElement)) return
  if (which === 'from') emit('days', event.target.value, props.to)
  else emit('days', props.from, event.target.value)
}
</script>

<template>
  <div class="picker">
    <BaseSelect
      v-model="choice"
      label="Período"
      :options="presetLabels"
      :aria-describedby="error ? errorId : undefined"
    />
    <template v-if="preset === 'custom'">
      <div class="day">
        <label :for="fromId">De</label>
        <input
          :id="fromId"
          type="date"
          :value="from"
          :aria-invalid="error ? 'true' : undefined"
          :aria-describedby="error ? errorId : undefined"
          @change="typed($event, 'from')"
        >
      </div>
      <div class="day">
        <label :for="toId">Até</label>
        <input
          :id="toId"
          type="date"
          :value="to"
          :aria-invalid="error ? 'true' : undefined"
          :aria-describedby="error ? errorId : undefined"
          @change="typed($event, 'to')"
        >
      </div>
    </template>
    <p
      v-if="error"
      :id="errorId"
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
  align-items: flex-end;
  gap: 12px;
}
.day {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.error {
  flex-basis: 100%;
  margin: 0;
  font-weight: 600;
  color: var(--enc-status-alert-text);
}
.day label {
  font-weight: 600;
}
.day input {
  height: var(--enc-control-height, 32px);
  padding: 0 8px;
  border: 1px solid var(--enc-secondary-border);
  border-radius: var(--enc-radius-card);
  background: var(--enc-surface);
  color: var(--enc-ink);
  font: inherit;
}

/* Phone: every field takes the full width. */
@media (max-width: 640px) {
  .picker,
  .day {
    width: 100%;
  }
  .picker > * {
    flex: 1 1 100%;
  }
}
</style>
