<script setup lang="ts">
/**
 * NeedsYou is "Precisa de você": the server's list, most urgent first, shown
 * as it came. Each entry names the order and the customer and opens the
 * order. The sentence depends on the kind; the days and the amount are the
 * server's. Entries for a late or at-risk item carry the same alert tag the
 * order list shows. Purchasing's entries, for the roles that buy, come first
 * in the order the server sent them and each opens Compras.
 */
import type { PurchasingNeed, Today } from '@encomendo/api'
import { computed } from 'vue'
import { type RouteLocationRaw, RouterLink } from 'vue-router'

import { formatDays, formatMoney } from '../../../format'
import AlertBadge from '../../../components/AlertBadge.vue'

type Need = Today['needsYou'][number]

const props = defineProps<{
  needs: readonly Need[]
  /** purchasing is Compras' entries: notices first, then pedidos de compra with no promised date, then items to send. */
  purchasing: readonly PurchasingNeed[]
}>()

/** sentence is the product's wording for a need (docs/product/design-document.md, Hoje). */
function sentence(need: Need): string {
  const item = need.itemName ?? 'Um item'
  switch (need.kind) {
    case 'late':
      return `${item} está atrasado.`
    case 'at_risk':
      return `${item} pode atrasar a entrega.`
    case 'awaiting_deposit':
      return `Aguardando o sinal há ${formatDays(need.days ?? 0)}.`
    case 'ready_not_handed_over':
      return `Pronto há ${formatDays(need.days ?? 0)} e ainda não entregue.`
    case 'refund_due':
      return need.amount === undefined ? 'Valor a devolver.' : `Valor a devolver: ${formatMoney(need.amount.cents, need.amount.currency)}.`
  }
}
/** purchasingSentence is the product's wording for a purchasing need (docs/product/design-document.md, Changes to Hoje). */
function purchasingSentence(need: PurchasingNeed): string {
  switch (need.kind) {
    case 'to_send':
      return `${need.count === 1 ? '1 item' : `${need.count} itens`} para comprar de ${need.supplierName}.`
    case 'no_promised_date':
      return `Pedido de compra ${need.number} sem prazo do fornecedor há ${formatDays(need.days)}.`
    case 'notify_supplier':
      return `Avise ${need.supplierName}: ${need.itemName} não é mais necessário (${need.number}).`
  }
}

/** purchasingTarget is where an entry leads: always inside Compras. A late pedido de compra opens its own screen, where "Informar prazo" is. */
function purchasingTarget(need: PurchasingNeed): RouteLocationRaw {
  return need.kind === 'no_promised_date' ? { name: 'supplier-order', params: { supplierOrderId: need.supplierOrderId } } : { name: 'purchasing' }
}

const empty = computed(() => props.needs.length === 0 && props.purchasing.length === 0)
</script>

<template>
  <section
    class="needs"
    aria-labelledby="needs-you"
  >
    <h2 id="needs-you">
      Precisa de você
    </h2>
    <p
      v-if="empty"
      class="muted"
    >
      Nada precisa de você agora.
    </p>
    <ul
      v-else
      class="list"
    >
      <li
        v-for="(need, index) in purchasing"
        :key="`purchasing-${need.kind}-${index}`"
      >
        <RouterLink
          class="entry"
          :to="purchasingTarget(need)"
        >
          <span class="what">{{ purchasingSentence(need) }}</span>
        </RouterLink>
      </li>
      <li
        v-for="(need, index) in needs"
        :key="`${need.kind}-${need.order.id}-${index}`"
      >
        <RouterLink
          class="entry"
          :to="{ name: 'order', params: { orderId: need.order.id } }"
        >
          <span class="who">
            <span class="enc-code">{{ need.order.number }}</span>{{ ' ' }}
            <span>{{ need.order.customer.name }}</span>
          </span>{{ ' ' }}
          <span class="what">
            <AlertBadge
              v-if="need.kind === 'late' || need.kind === 'at_risk'"
              :kind="need.kind"
            />{{ ' ' }}
            {{ sentence(need) }}
          </span>
        </RouterLink>
      </li>
    </ul>
  </section>
</template>

<style scoped>
.needs {
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
.list {
  display: flex;
  flex-direction: column;
  margin: 0;
  padding: 0;
  list-style: none;
}
.entry {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px 16px;
  padding: 8px 0;
  border-bottom: 1px solid var(--enc-hairline);
  color: var(--enc-ink);
  text-decoration: none;
}
.who {
  display: inline-flex;
  gap: 8px;
  font-weight: 600;
}
.what {
  display: inline-flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

/* Phone: the sentence drops under the order, and every entry is a thumb-sized target. */
@media (max-width: 640px) {
  .entry {
    flex-direction: column;
    align-items: flex-start;
    min-height: var(--enc-control-height);
  }
}
</style>
