<script setup lang="ts">
/**
 * SummaryCards are the four numbers at the top of Hoje. Each is a link that
 * opens Pedidos with the filter it counts: the numbers are the server's, and
 * nothing here adds or compares them. "Entregas nos próximos 7 dias" opens
 * the plain list, because the order list has no delivery-window filter yet.
 * "Compras a enviar" is a fifth card, for the roles that buy: the items in "A
 * comprar", opening Compras.
 */
import type { Today } from '@encomendo/api'
import { type RouteLocationRaw, RouterLink } from 'vue-router'

defineProps<{
  counts: Today['counts']
  /** toSend is the items in "A comprar", or null for a role that does not buy (no card). */
  toSend: number | null
}>()

const orders = (query: Record<string, string> = {}): RouteLocationRaw => ({ name: 'orders', query })
</script>

<template>
  <ul
    class="cards"
    :class="{ 'cards--five': toSend !== null }"
  >
    <li>
      <RouterLink
        class="card"
        :to="orders()"
      >
        <span class="label">Pedidos ativos</span>{{ ' ' }}
        <span class="value enc-number">{{ counts.active }}</span>
      </RouterLink>
    </li>
    <li>
      <RouterLink
        class="card"
        :to="orders({ phase: 'awaiting_deposit' })"
      >
        <span class="label">Aguardando sinal</span>{{ ' ' }}
        <span class="value enc-number">{{ counts.awaitingDeposit }}</span>
      </RouterLink>
    </li>
    <li>
      <RouterLink
        class="card"
        :to="orders({ alert: 'true' })"
      >
        <span class="label">Com alerta</span>{{ ' ' }}
        <span class="value enc-number">{{ counts.withAlert }}</span>
      </RouterLink>
    </li>
    <li>
      <RouterLink
        class="card"
        :to="orders()"
      >
        <span class="label">Entregas nos próximos 7 dias</span>{{ ' ' }}
        <span class="value enc-number">{{ counts.dueIn7Days }}</span>
      </RouterLink>
    </li>
    <li v-if="toSend !== null">
      <RouterLink
        class="card"
        :to="{ name: 'purchasing' }"
      >
        <span class="label">Compras a enviar</span>{{ ' ' }}
        <span class="value enc-number">{{ toSend }}</span>
      </RouterLink>
    </li>
  </ul>
</template>

<style scoped>
.cards {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
  margin: 0;
  padding: 0;
  list-style: none;
}
.cards--five {
  grid-template-columns: repeat(5, minmax(0, 1fr));
}
.card {
  display: flex;
  flex-direction: column;
  gap: 4px;
  height: 100%;
  box-sizing: border-box;
  padding: 12px 16px;
  border: 1px solid var(--enc-hairline);
  border-radius: var(--enc-radius-card);
  background: var(--enc-surface);
  color: var(--enc-ink);
  text-decoration: none;
}
.label {
  color: var(--enc-ink-secondary);
}
.value {
  font-size: 28px;
  line-height: 32px;
}

/* Phone: two cards a row, each a thumb-sized target. */
@media (max-width: 640px) {
  .cards,
  .cards--five {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
  .card {
    min-height: var(--enc-control-height);
  }
}
</style>
