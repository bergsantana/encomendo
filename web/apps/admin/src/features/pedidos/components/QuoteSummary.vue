<script setup lang="ts">
/**
 * QuoteSummary is "Resumo": Itens, Frete, Desconto, Total, Sinal and
 * "Entrega prevista" (with its days after the deposit), then "Custo" and "Margem" when the API
 * sent them (the owner only; "—" when it has no cost to give), exactly as `POST /orders/quote` answered. It adds
 * nothing up and shows "Calculando…" while the next answer is awaited. A
 * refusal of the quote (an invalid field, an archived item) shows its message
 * in place of the figures.
 */
import type { Quote } from '@encomendo/api'

import { formatCivilDate, formatDays, formatMargin, formatMoney } from '../../../format'

defineProps<{
  quote: Quote | null
  quoting: boolean
  error: string | null
}>()
</script>

<template>
  <section
    class="summary"
    aria-labelledby="summary-heading"
  >
    <h2 id="summary-heading">
      Resumo
    </h2>
    <p
      v-if="quoting"
      role="status"
    >
      Calculando…
    </p>
    <p
      v-else-if="error"
      role="alert"
      class="error"
    >
      {{ error }}
    </p>
    <dl v-else-if="quote">
      <div>
        <dt>Itens</dt>
        <dd class="enc-number">
          {{ formatMoney(quote.subtotal.cents, quote.subtotal.currency) }}
        </dd>
      </div>
      <div>
        <dt>Frete</dt>
        <dd class="enc-number">
          {{ formatMoney(quote.freight.cents, quote.freight.currency) }}
        </dd>
      </div>
      <div>
        <dt>Desconto</dt>
        <dd class="enc-number">
          −{{ formatMoney(quote.discount.cents, quote.discount.currency) }}
        </dd>
      </div>
      <div class="total">
        <dt>Total</dt>
        <dd class="enc-number">
          {{ formatMoney(quote.total.cents, quote.total.currency) }}
        </dd>
      </div>
      <div>
        <dt>Sinal ({{ quote.depositPercent }}%)</dt>
        <dd class="enc-number">
          {{ formatMoney(quote.depositDue.cents, quote.depositDue.currency) }}
        </dd>
      </div>
      <div>
        <dt>Entrega prevista</dt>
        <dd class="enc-number">
          {{ formatCivilDate(quote.deliveryDate) }} ({{ formatDays(quote.daysAfterDeposit) }} depois do sinal)
        </dd>
      </div>
      <div v-if="quote.cost !== undefined">
        <dt>Custo</dt>
        <dd class="enc-number">
          {{ quote.cost === null ? '—' : formatMoney(quote.cost.cents, quote.cost.currency) }}
        </dd>
      </div>
      <div v-if="quote.margin !== undefined">
        <dt>Margem</dt>
        <dd class="enc-number">
          {{ quote.margin === null ? '—' : formatMargin(quote.margin, quote.total.currency) }}
        </dd>
      </div>
    </dl>
    <p
      v-else
      class="muted"
    >
      Os valores aparecem quando o pedido tiver itens.
    </p>
  </section>
</template>

<style scoped>
.summary {
  display: flex;
  flex-direction: column;
  gap: 8px;
  max-width: 420px;
  padding: 12px;
  border: 1px solid var(--enc-hairline);
  border-radius: var(--enc-radius-card);
  background: var(--enc-surface);
}
h2,
p {
  margin: 0;
}
h2 {
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
  justify-content: space-between;
  gap: 12px;
}
dt {
  color: var(--enc-ink-secondary);
}
dd {
  margin: 0;
}
.total {
  padding-top: 4px;
  border-top: 1px solid var(--enc-hairline);
}
.muted {
  color: var(--enc-ink-secondary);
}
.error {
  color: var(--enc-status-alert-text);
  font-weight: 600;
}

@media (max-width: 640px) {
  .summary {
    max-width: none;
  }
}
</style>
