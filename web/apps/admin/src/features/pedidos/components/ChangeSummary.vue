<script setup lang="ts">
/**
 * ChangeSummary is what an "Alterar pedido" changes: "Total: {antes} →
 * {depois}" and "Entrega prevista: {antes} → {depois}". Before saving it
 * compares the order with the quote's answer; after saving it shows what the
 * API recorded, and "Valor a devolver" when the new total is below what was
 * paid (the refund happens outside Encomendo). Every figure is the API's.
 */
import type { ChangedOrder, Order, Quote } from '@encomendo/api'
import { computed } from 'vue'

import { formatCivilDate, formatMoney } from '../../../format'

const props = defineProps<{
  order: Order
  quote: Quote | null
  /** changed is the answer to the save; the figures then come from it. */
  changed: ChangedOrder | null
}>()

const change = computed(() => props.changed?.change ?? null)
</script>

<template>
  <section
    v-if="quote || change"
    class="change"
    aria-labelledby="change-heading"
  >
    <h2 id="change-heading">
      O que muda
    </h2>
    <dl v-if="change">
      <div>
        <dt>Total</dt>
        <dd class="enc-number">
          {{ formatMoney(change.oldTotal.cents, change.oldTotal.currency) }} → {{ formatMoney(change.newTotal.cents, change.newTotal.currency) }}
        </dd>
      </div>
      <div>
        <dt>Entrega prevista</dt>
        <dd class="enc-number">
          {{ formatCivilDate(change.oldDeliveryDate) }} → {{ formatCivilDate(change.newDeliveryDate) }}
        </dd>
      </div>
      <div v-if="change.refundDue.cents > 0">
        <dt>Valor a devolver</dt>
        <dd class="enc-number">
          {{ formatMoney(change.refundDue.cents, change.refundDue.currency) }}
        </dd>
      </div>
    </dl>
    <dl v-else-if="quote">
      <div>
        <dt>Total</dt>
        <dd class="enc-number">
          {{ formatMoney(order.total.cents, order.total.currency) }} → {{ formatMoney(quote.total.cents, quote.total.currency) }}
        </dd>
      </div>
      <div>
        <dt>Entrega prevista</dt>
        <dd class="enc-number">
          {{ formatCivilDate(order.deliveryDate) }} → {{ formatCivilDate(quote.deliveryDate) }}
        </dd>
      </div>
      <div v-if="quote.refundDue.cents > 0">
        <dt>Valor a devolver</dt>
        <dd class="enc-number">
          {{ formatMoney(quote.refundDue.cents, quote.refundDue.currency) }}
        </dd>
      </div>
    </dl>
  </section>
</template>

<style scoped>
.change {
  display: flex;
  flex-direction: column;
  gap: 8px;
  max-width: 420px;
  padding: 12px;
  border: 1px solid var(--enc-hairline);
  border-radius: var(--enc-radius-card);
  background: var(--enc-surface);
}
h2 {
  margin: 0;
  font-size: 15px;
}
dl {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin: 0;
}
dl div {
  display: flex;
  flex-wrap: wrap;
  justify-content: space-between;
  gap: 4px 12px;
}
dt {
  color: var(--enc-ink-secondary);
}
dd {
  margin: 0;
}

@media (max-width: 640px) {
  .change {
    max-width: none;
  }
}
</style>
