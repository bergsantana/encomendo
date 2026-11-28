<script setup lang="ts">
/**
 * ReceivedPanel is "Recebido no período": the total the server counted, then
 * the same money by type ("Sinal", "Saldo") and by form ("Pix", "Dinheiro"…),
 * each line as the server sent it. Voided payments are not in it. A period
 * with nothing says "Nenhum pagamento neste período."
 */
import type { FinanceSummary } from '@encomendo/api'

import { formatMoney } from '../../../format'
import { paymentKindLabel, paymentMethodLabel } from '../../../paymentLabels'

const props = defineProps<{
  received: FinanceSummary['received']
}>()

const empty = (): boolean => props.received.byKind.length === 0 && props.received.byMethod.length === 0
</script>

<template>
  <section
    class="received"
    aria-labelledby="received-title"
  >
    <h2 id="received-title">
      Recebido no período
    </h2>
    <p
      v-if="empty()"
      class="muted"
    >
      Nenhum pagamento neste período.
    </p>
    <template v-else>
      <p class="total enc-number">
        {{ formatMoney(received.total.cents, received.total.currency) }}
      </p>
      <div class="groups">
        <table>
          <caption>
            Por tipo
          </caption>
          <tbody>
            <tr
              v-for="entry in received.byKind"
              :key="entry.key"
            >
              <th scope="row">
                {{ paymentKindLabel(entry.key) }}
              </th>
              <td class="enc-number">
                {{ formatMoney(entry.total.cents, entry.total.currency) }}
              </td>
            </tr>
          </tbody>
        </table>
        <table>
          <caption>
            Por forma
          </caption>
          <tbody>
            <tr
              v-for="entry in received.byMethod"
              :key="entry.key"
            >
              <th scope="row">
                {{ paymentMethodLabel(entry.key) }}
              </th>
              <td class="enc-number">
                {{ formatMoney(entry.total.cents, entry.total.currency) }}
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </template>
  </section>
</template>

<style scoped>
.received {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
h2,
p {
  margin: 0;
}
h2 {
  font-size: 15px;
}
.muted {
  color: var(--enc-ink-secondary);
}
.total {
  font-size: 28px;
  line-height: 32px;
}
.groups {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 48px;
}
table {
  min-width: 240px;
  border-collapse: collapse;
  font-family: var(--enc-font-condensed);
  font-size: var(--enc-table-size, 13px);
}
caption {
  padding: 4px 0;
  color: var(--enc-ink-secondary);
  text-align: left;
}
th,
td {
  padding: 6px 12px 6px 0;
  border-bottom: 1px solid var(--enc-hairline);
  text-align: left;
  font-weight: 400;
}
td {
  text-align: right;
}

/* Phone: the two tables stack and take the width. */
@media (max-width: 640px) {
  .groups {
    flex-direction: column;
  }
  table {
    width: 100%;
  }
}
</style>
