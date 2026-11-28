<script setup lang="ts">
/**
 * PaymentDialog is "Registrar pagamento": "Tipo" (Sinal or Saldo), "Valor"
 * (filled with what is due) and "Forma". The amount is typed as text and
 * becomes cents only when sent. What the API refuses (an amount above what is
 * still due) shows above the form. A sheet on the phone, with its own buttons.
 */
import type { Order } from '@encomendo/api'
import { BaseButton, BaseDialog, BaseInput, BaseSelect } from '@encomendo/ui'
import { ref, watch } from 'vue'

import { centsToText, parseCents } from '../../../format'
import { type PaymentKind, type PaymentMethod, paymentMethods } from '../labels'
import type { PaymentInput } from '../orderStore'

const props = defineProps<{
  order: Order
  error: string | null
  busy: boolean
}>()

const emit = defineEmits<{
  submit: [payment: PaymentInput]
  close: []
}>()

const kind = ref<PaymentKind>(props.order.depositPaid ? 'balance' : 'deposit')
const amount = ref('')
const method = ref<PaymentMethod>('pix')
const amountError = ref<string | null>(null)

function due(chosen: PaymentKind): string {
  const value = chosen === 'deposit' ? props.order.depositDue : props.order.balanceDue
  return centsToText(value.cents)
}

amount.value = due(kind.value)
// The amount follows the type until the person types their own.
watch(kind, (chosen) => {
  amount.value = due(chosen)
})

function isMethod(value: string): value is PaymentMethod {
  return paymentMethods.some((choice) => choice.value === value)
}

function submit(): void {
  const cents = parseCents(amount.value)
  if (cents === null || cents <= 0) {
    amountError.value = 'Informe um valor maior que zero.'
    return
  }
  amountError.value = null
  emit('submit', { kind: kind.value, amountCents: cents, method: method.value })
}
</script>

<template>
  <BaseDialog
    title="Registrar pagamento"
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
      <BaseSelect
        :model-value="kind"
        label="Tipo"
        :options="[
          { value: 'deposit', label: 'Sinal' },
          { value: 'balance', label: 'Saldo' },
        ]"
        @update:model-value="kind = $event === 'balance' ? 'balance' : 'deposit'"
      />
      <BaseInput
        v-model="amount"
        label="Valor"
        inputmode="decimal"
        :error="amountError"
      />
      <BaseSelect
        :model-value="method"
        label="Forma"
        :options="paymentMethods"
        @update:model-value="(value) => isMethod(value) && (method = value)"
      />
      <div class="buttons enc-sheet-actions">
        <BaseButton
          variant="primary"
          type="submit"
          :disabled="busy"
        >
          Registrar
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
.buttons {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
</style>
