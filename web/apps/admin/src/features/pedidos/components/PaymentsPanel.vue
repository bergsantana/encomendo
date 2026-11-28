<script setup lang="ts">
/**
 * PaymentsPanel is "Pagamentos": Total, Sinal, Pago, Saldo and "Valor a
 * devolver" as the API computed them (and "Custo" and "Margem" only when it
 * sent them, which is for the owner), the payments with "Anulado" on a voided
 * one, and the buttons: "Registrar pagamento", "Anular" (owner) and "Liberar
 * sem sinal" (owner, while the deposit is awaited).
 */
import type { Order } from '@encomendo/api'
import { BaseButton } from '@encomendo/ui'

import { formatDate, formatMargin, formatMoney } from '../../../format'
import { paymentKindLabel, paymentMethodLabel } from '../labels'

defineProps<{
  order: Order
  timeZone?: string
  canRecord: boolean
  canVoid: boolean
  canRelease: boolean
  busy: boolean
}>()

const emit = defineEmits<{
  record: []
  void: [paymentId: string]
  release: []
}>()

const money = (value: { cents: number; currency: string }) => formatMoney(value.cents, value.currency)
</script>

<template>
  <section
    class="payments"
    aria-labelledby="payments-heading"
  >
    <h2 id="payments-heading">
      Pagamentos
    </h2>
    <dl>
      <div>
        <dt>Total</dt>
        <dd class="enc-number">
          {{ money(order.total) }}
        </dd>
      </div>
      <div>
        <dt>Sinal</dt>
        <dd class="enc-number">
          {{ money(order.depositDue) }} ({{ order.depositPercent }}%)
        </dd>
      </div>
      <div>
        <dt>Pago</dt>
        <dd class="enc-number">
          {{ money(order.paid) }}
        </dd>
      </div>
      <div>
        <dt>Saldo</dt>
        <dd class="enc-number">
          {{ money(order.balanceDue) }}
        </dd>
      </div>
      <div v-if="order.refundDue.cents > 0">
        <dt>Valor a devolver</dt>
        <dd class="enc-number">
          {{ money(order.refundDue) }}
        </dd>
      </div>
      <div v-if="order.cost">
        <dt>Custo</dt>
        <dd class="enc-number">
          {{ money(order.cost) }}
        </dd>
      </div>
      <div v-if="order.margin">
        <dt>Margem</dt>
        <dd class="enc-number">
          {{ formatMargin(order.margin, order.total.currency) }}
        </dd>
      </div>
    </dl>

    <table
      v-if="order.payments.length > 0"
      class="enc-cards"
    >
      <caption class="enc-visually-hidden">
        Pagamentos do pedido
      </caption>
      <thead>
        <tr>
          <th scope="col">
            Data
          </th>
          <th scope="col">
            Tipo
          </th>
          <th scope="col">
            Forma
          </th>
          <th
            scope="col"
            class="number"
          >
            Valor
          </th>
          <th scope="col">
            Registrado por
          </th>
          <th scope="col">
            Ações
          </th>
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="payment in order.payments"
          :key="payment.id"
          :data-voided="payment.voided ? 'true' : 'false'"
        >
          <td data-label="Data">
            {{ formatDate(payment.recordedAt, timeZone) }}
          </td>
          <td data-label="Tipo">
            {{ paymentKindLabel(payment.kind) }}
            <span
              v-if="payment.voided"
              class="voided"
            >Anulado<span
              v-if="payment.voided.reason"
              class="reason"
            >: {{ payment.voided.reason }}</span></span>
          </td>
          <td data-label="Forma">
            {{ paymentMethodLabel(payment.method) }}
          </td>
          <td
            class="number enc-number"
            data-label="Valor"
          >
            {{ money(payment.amount) }}
          </td>
          <td data-label="Registrado por">
            {{ payment.recordedBy.name }}
          </td>
          <td class="enc-cards__actions">
            <BaseButton
              v-if="canVoid && !payment.voided"
              :disabled="busy"
              :aria-label="`Anular o pagamento de ${money(payment.amount)}`"
              @click="emit('void', payment.id)"
            >
              Anular
            </BaseButton>
          </td>
        </tr>
      </tbody>
    </table>

    <div class="buttons">
      <BaseButton
        v-if="canRecord"
        :disabled="busy"
        @click="emit('record')"
      >
        Registrar pagamento
      </BaseButton>
      <BaseButton
        v-if="canRelease"
        :disabled="busy"
        @click="emit('release')"
      >
        Liberar sem sinal
      </BaseButton>
    </div>
  </section>
</template>

<style scoped>
.payments {
  display: flex;
  flex-direction: column;
  gap: 8px;
  width: 100%;
}
h2 {
  margin: 0;
  font-size: 15px;
}
dl {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 24px;
  margin: 0;
}
dl div {
  display: flex;
  gap: 8px;
}
dt {
  color: var(--enc-ink-secondary);
}
dd {
  margin: 0;
}
table {
  width: 100%;
  border-collapse: collapse;
  font-family: var(--enc-font-condensed);
  font-size: var(--enc-table-size, 13px);
}
th,
td {
  padding: 8px 12px;
  border-bottom: 1px solid var(--enc-hairline);
  text-align: left;
}
thead th {
  color: var(--enc-ink-secondary);
}
.number {
  text-align: right;
}
tr[data-voided='true'] .number {
  text-decoration: line-through;
}
.voided {
  display: inline-block;
  margin-left: 6px;
  padding: 0 6px;
  border-radius: var(--enc-radius-badge);
  background: var(--enc-status-neutral-fill);
  box-shadow: inset 0 0 0 1px var(--enc-secondary-border);
  color: var(--enc-ink-secondary);
  font-weight: 600;
}
.reason {
  font-weight: 400;
}
.buttons {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

@media (max-width: 640px) {
  dl {
    flex-direction: column;
  }
  .number {
    text-align: left;
  }
  .buttons {
    flex-direction: column;
  }
}
</style>
