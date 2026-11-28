<script setup lang="ts">
/**
 * SupplierOrderLines is "Itens" of a pedido de compra: item, "Qtd", the
 * client's order, "Precisa chegar até", "Previsto" and "Situação", with
 * "Pode atrasar" on a line the forecast pushes past its limit. "Custo (por
 * unidade)" and "Preço pago (por unidade)" are columns only when the API sent
 * them, which is for the owner alone. Dates and alerts are the API's. On the
 * phone each line is a card.
 */
import type { PurchaseLine } from '@encomendo/api'
import { IconAlertTriangle, StatusBadge } from '@encomendo/ui'
import { computed } from 'vue'
import { RouterLink } from 'vue-router'

import AlertBadge from '../../../components/AlertBadge.vue'
import { formatCivilDate, formatMoney } from '../../../format'
import { choicesText, lineAlert, lineStatusLabel, lineTone } from '../labels'

const props = defineProps<{
  lines: readonly PurchaseLine[]
}>()

const showCost = computed(() => props.lines.some((line) => line.cost !== undefined))
const showPaid = computed(() => props.lines.some((line) => line.paidCost !== undefined && line.paidCost !== null))

function day(value: string | null): string {
  return value === null ? '—' : formatCivilDate(value)
}
</script>

<template>
  <table class="enc-cards">
    <caption class="enc-visually-hidden">
      Itens do pedido de compra
    </caption>
    <thead>
      <tr>
        <th scope="col">
          Item
        </th>
        <th scope="col">
          Qtd
        </th>
        <th scope="col">
          Pedido
        </th>
        <th scope="col">
          Precisa chegar até
        </th>
        <th scope="col">
          Previsto
        </th>
        <th scope="col">
          Situação
        </th>
        <th
          v-if="showCost"
          scope="col"
        >
          Custo (por unidade)
        </th>
        <th
          v-if="showPaid"
          scope="col"
        >
          Preço pago (por unidade)
        </th>
      </tr>
    </thead>
    <tbody>
      <tr
        v-for="line in lines"
        :key="line.id"
      >
        <th scope="row">
          {{ line.name }}
          <span
            v-if="line.choices.length > 0"
            class="muted"
          >{{ choicesText(line.choices) }}</span>
          <span
            v-if="line.checkNote"
            class="muted"
          >{{ line.checkNote }}</span>
        </th>
        <td
          data-label="Qtd"
          class="enc-number"
        >
          {{ line.quantity }}
        </td>
        <td data-label="Pedido">
          <RouterLink
            class="enc-code"
            :to="{ name: 'order', params: { orderId: line.orderId } }"
          >
            {{ line.orderNumber }}
          </RouterLink>
        </td>
        <td data-label="Precisa chegar até">
          {{ day(line.arrivalLimit) }}
        </td>
        <td data-label="Previsto">
          {{ day(line.expectedDate) }}
          <AlertBadge
            v-if="lineAlert(line)"
            :kind="lineAlert(line) ?? 'at_risk'"
          />
        </td>
        <td data-label="Situação">
          <span
            v-if="line.notice"
            class="notice"
          >
            <IconAlertTriangle
              :size="14"
              stroke-width="2"
              aria-hidden="true"
            />
            Avisar o fornecedor
          </span>
          <StatusBadge
            v-else
            :label="lineStatusLabel(line.status)"
            :phase="lineTone(line.status)"
          />
        </td>
        <td
          v-if="showCost"
          data-label="Custo (por unidade)"
          class="enc-number"
        >
          {{ line.cost ? formatMoney(line.cost.cents, line.cost.currency) : '—' }}
        </td>
        <td
          v-if="showPaid"
          data-label="Preço pago (por unidade)"
          class="enc-number"
        >
          {{ line.paidCost ? formatMoney(line.paidCost.cents, line.paidCost.currency) : '—' }}
        </td>
      </tr>
    </tbody>
  </table>
</template>

<style scoped>
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
  vertical-align: top;
}
thead th {
  color: var(--enc-ink-secondary);
}
tbody th {
  font-weight: 600;
}
.muted {
  display: block;
  color: var(--enc-ink-secondary);
  font-weight: 400;
}
.notice {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 2px 6px;
  border-radius: var(--enc-radius-badge);
  background: var(--enc-status-alert-fill);
  color: var(--enc-status-alert-text);
  font-size: 12px;
  font-weight: 600;
  line-height: 16px;
}

/* Phone: the cards of enc-cards (tokens.css); the order link is a 48 px target. */
@media (max-width: 640px) {
  td a {
    display: inline-flex;
    align-items: center;
    min-height: var(--enc-control-height);
  }
}
</style>
