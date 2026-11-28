<script setup lang="ts">
/**
 * MarginsTable is "Margem dos pedidos entregues": each order handed over in
 * the period with its total, cost and margin as the server computed them, "—"
 * where an item's cost was not known, and the period's total from the server.
 * Nothing is added up here.
 */
import type { FinanceSummary } from '@encomendo/api'
import { RouterLink } from 'vue-router'

import { formatCivilDate, formatMargin, formatMoney } from '../../../format'

defineProps<{
  margins: FinanceSummary['margins']
  totals: FinanceSummary['marginsTotal']
}>()
</script>

<template>
  <section
    class="margins"
    aria-labelledby="margins-title"
  >
    <h2 id="margins-title">
      Margem dos pedidos entregues
    </h2>
    <p
      v-if="margins.length === 0"
      class="muted"
    >
      Nenhum pedido entregue neste período.
    </p>
    <template v-else>
      <table class="enc-cards">
        <caption class="enc-visually-hidden">
          Pedidos entregues no período
        </caption>
        <thead>
          <tr>
            <th scope="col">
              Pedido
            </th>
            <th scope="col">
              Entregue em
            </th>
            <th
              scope="col"
              class="money"
            >
              Total
            </th>
            <th
              scope="col"
              class="money"
            >
              Custo
            </th>
            <th
              scope="col"
              class="money"
            >
              Margem
            </th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="row in margins"
            :key="row.orderId"
          >
            <th scope="row">
              <RouterLink
                class="number enc-code"
                :to="{ name: 'order', params: { orderId: row.orderId } }"
              >
                {{ row.number }}
              </RouterLink>{{ ' ' }}
              <span class="customer">{{ row.customerName }}</span>
            </th>
            <td data-label="Entregue em">
              {{ formatCivilDate(row.handedOverOn) }}
            </td>
            <td
              class="money enc-number"
              data-label="Total"
            >
              {{ formatMoney(row.total.cents, row.total.currency) }}
            </td>
            <td
              class="money enc-number"
              data-label="Custo"
            >
              {{ row.cost == null ? '—' : formatMoney(row.cost.cents, row.cost.currency) }}
            </td>
            <td
              class="money enc-number"
              data-label="Margem"
            >
              {{ row.margin == null ? '—' : formatMargin(row.margin, row.total.currency) }}
            </td>
          </tr>
        </tbody>
        <tfoot>
          <tr>
            <th scope="row">
              Total do período
            </th>
            <td />
            <td
              class="money enc-number"
              data-label="Total"
            >
              {{ formatMoney(totals.total.cents, totals.total.currency) }}
            </td>
            <td
              class="money enc-number"
              data-label="Custo"
            >
              {{ formatMoney(totals.cost.cents, totals.cost.currency) }}
            </td>
            <td
              class="money enc-number"
              data-label="Margem"
            >
              {{ totals.margin == null ? '—' : formatMargin(totals.margin, totals.total.currency) }}
            </td>
          </tr>
        </tfoot>
      </table>
      <p class="muted">
        Custo e margem somam só os pedidos com todos os custos informados.
      </p>
    </template>
  </section>
</template>

<style scoped>
.margins {
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
tbody th {
  font-weight: 400;
}
tfoot th {
  font-weight: 600;
}
.money {
  text-align: right;
}
.number {
  color: var(--enc-ink);
  font-weight: 600;
}
.customer {
  display: block;
  color: var(--enc-ink-secondary);
}

/*
 * Phone: the cards of enc-cards (tokens.css) for the rows; the period's total
 * is a card of its own, in a footer that enc-cards does not restyle.
 */
@media (max-width: 640px) {
  .money {
    text-align: left;
  }
  .number {
    display: flex;
    align-items: center;
    min-height: var(--enc-control-height);
  }
  table.enc-cards tfoot {
    display: block;
  }
  table.enc-cards tfoot tr {
    display: flex;
    flex-direction: column;
    gap: 8px;
    padding: 12px;
    border: 1px solid var(--enc-hairline);
    border-radius: var(--enc-radius-card);
    background: var(--enc-surface);
  }
  table.enc-cards tfoot td,
  table.enc-cards tfoot th {
    display: block;
    padding: 0;
    border: 0;
  }
  table.enc-cards tfoot td:not([data-label]) {
    display: none;
  }
  table.enc-cards tfoot td[data-label]::before {
    content: attr(data-label);
    display: block;
    color: var(--enc-ink-secondary);
    font-size: 12px;
    font-weight: 400;
  }
}
</style>
