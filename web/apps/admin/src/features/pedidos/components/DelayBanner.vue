<script setup lang="ts">
/**
 * DelayBanner is the warning above the order's items: "{n} item pode atrasar
 * a entrega", "{n} itens podem atrasar a entrega", "{n} item atrasado" or
 * "{n} itens atrasados", then each item's expected date against "Precisa
 * chegar até". Its ways out are buttons: "Usar o fornecedor reserva" (while
 * the item is still "A comprar" and has a backup), "Falar com o fornecedor
 * ({fornecedor})" (only for an item the API sent a chat link for, which it
 * does for the roles that buy) and "Alterar a data de entrega". Which items
 * have an alert is the API's call.
 */
import type { Order, OrderPurchase } from '@encomendo/api'
import { BaseButton } from '@encomendo/ui'
import { computed } from 'vue'

import AlertBadge from '../../../components/AlertBadge.vue'
import { formatCivilDate, formatDays } from '../../../format'
import type { OrderItem } from '../labels'

const props = defineProps<{
  order: Order
  /** canFix is `order:manage`: the roles that may switch the supplier or change the date. */
  canFix: boolean
  busy: boolean
  /** purchases is what purchasing knows of the bought items, by order item: where the supplier chat link comes from. */
  purchases: Readonly<Record<string, OrderPurchase>>
}>()

const emit = defineEmits<{
  useBackup: [itemId: string]
  changeDate: []
  talkToSupplier: [itemId: string]
}>()

const late = computed(() => props.order.items.filter((item) => item.alerts.includes('late')))
const atRisk = computed(() => props.order.items.filter((item) => item.alerts.length > 0 && !item.alerts.includes('late')))
const flagged = computed(() => [...late.value, ...atRisk.value])

function lateTitle(count: number): string {
  return count === 1 ? '1 item atrasado' : `${count} itens atrasados`
}

function riskTitle(count: number): string {
  return count === 1 ? '1 item pode atrasar a entrega' : `${count} itens podem atrasar a entrega`
}

/** supplierName is the supplier of a delayed item, when the API sent the chat link for it. */
function chatSupplier(item: OrderItem): string | null {
  const purchase = props.purchases[item.id]
  return purchase?.supplierChatUrl === undefined ? null : purchase.supplierName
}

function canSwitch(item: OrderItem): boolean {
  return props.canFix && item.status === 'to_buy' && item.backupSupplier !== undefined
}
</script>

<template>
  <section
    v-if="flagged.length > 0"
    class="banner"
    aria-labelledby="delay-heading"
  >
    <h2 id="delay-heading">
      <span v-if="late.length > 0">{{ lateTitle(late.length) }}</span>
      <span v-if="atRisk.length > 0">{{ riskTitle(atRisk.length) }}</span>
    </h2>
    <ul>
      <li
        v-for="item in flagged"
        :key="item.id"
      >
        <AlertBadge :kind="item.alerts.includes('late') ? 'late' : 'at_risk'" />
        <p>{{ item.name }}: previsto para {{ formatCivilDate(item.expectedDate) }}, precisa chegar até {{ formatCivilDate(order.arrivalLimit) }}.</p>
        <BaseButton
          v-if="canSwitch(item) && item.backupSupplier"
          :disabled="busy"
          @click="emit('useBackup', item.id)"
        >
          Usar o fornecedor reserva ({{ item.backupSupplier.name }}, {{ formatDays(item.backupSupplier.leadDays) }})
        </BaseButton>
        <BaseButton
          v-if="chatSupplier(item) !== null"
          :disabled="busy"
          @click="emit('talkToSupplier', item.id)"
        >
          Falar com o fornecedor ({{ chatSupplier(item) }})
        </BaseButton>
      </li>
    </ul>
    <div v-if="canFix">
      <BaseButton @click="emit('changeDate')">
        Alterar a data de entrega
      </BaseButton>
    </div>
  </section>
</template>

<style scoped>
.banner {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 12px;
  border-radius: var(--enc-radius-card);
  background: var(--enc-status-alert-fill);
  color: var(--enc-status-alert-text);
}
h2 {
  display: flex;
  flex-direction: column;
  margin: 0;
  font-size: 15px;
}
ul {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin: 0;
  padding: 0;
  list-style: none;
}
li {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 4px;
}
p {
  margin: 0;
}

@media (max-width: 640px) {
  li {
    align-items: stretch;
  }
  .banner > div {
    display: flex;
    flex-direction: column;
  }
}
</style>
