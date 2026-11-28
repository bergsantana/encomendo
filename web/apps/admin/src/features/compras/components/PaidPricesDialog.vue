<script setup lang="ts">
/**
 * PaidPricesDialog is "Informar preço pago" (the owner's): one field per
 * line, "Preço pago (por unidade)", filled with the cost at order time or the
 * price already recorded, and "Salvar preços". Typed as "89,90" and sent as
 * integer cents, never as a float. A sheet on the phone.
 */
import type { PurchaseLine } from '@encomendo/api'
import { BaseButton, BaseDialog, BaseInput } from '@encomendo/ui'
import { ref } from 'vue'

import { centsToText, parseCents } from '../../../format'

const props = defineProps<{
  /** lines are the lines that are not cancelled. */
  lines: readonly PurchaseLine[]
  error: string | null
  busy: boolean
}>()

const emit = defineEmits<{
  submit: [prices: { lineId: string; cents: number }[]]
  close: []
}>()

function startingText(line: PurchaseLine): string {
  const known = line.paidCost ?? line.cost
  return known === undefined ? '' : centsToText(known.cents)
}

const texts = ref<Record<string, string>>(Object.fromEntries(props.lines.map((line) => [line.id, startingText(line)])))
const wrong = ref<Record<string, boolean>>({})

function submit(): void {
  const prices: { lineId: string; cents: number }[] = []
  const bad: Record<string, boolean> = {}
  for (const line of props.lines) {
    const cents = parseCents(texts.value[line.id] ?? '')
    if (cents === null) bad[line.id] = true
    else prices.push({ lineId: line.id, cents })
  }
  wrong.value = bad
  if (Object.keys(bad).length === 0) emit('submit', prices)
}
</script>

<template>
  <BaseDialog
    title="Informar preço pago"
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
      <fieldset
        v-for="line in lines"
        :key="line.id"
        class="line"
      >
        <legend class="enc-number">
          {{ line.quantity }}× {{ line.name }}
        </legend>
        <BaseInput
          :model-value="texts[line.id] ?? ''"
          label="Preço pago (por unidade)"
          inputmode="decimal"
          :error="wrong[line.id] ? 'Confira o valor.' : null"
          @update:model-value="(value) => (texts = { ...texts, [line.id]: value })"
        />
      </fieldset>
      <div class="buttons enc-sheet-actions">
        <BaseButton
          variant="primary"
          type="submit"
          :disabled="busy"
        >
          Salvar preços
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
.line {
  margin: 0;
  padding: 12px;
  border: 1px solid var(--enc-hairline);
  border-radius: var(--enc-radius-card);
}
legend {
  padding: 0 4px;
}
.buttons {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
</style>
