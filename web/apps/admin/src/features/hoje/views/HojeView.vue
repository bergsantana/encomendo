<script setup lang="ts">
/**
 * HojeView is the first screen: the store's day with four summary cards,
 * "Precisa de você" and "Próximas entregas". Every number and list is the
 * server's answer for today in the store's time zone; this screen only lays
 * it out. Today's purchases come with Compras (milestone 5).
 */
import { BaseButton } from '@encomendo/ui'
import { storeToRefs } from 'pinia'
import { onMounted } from 'vue'
import { RouterLink } from 'vue-router'

import { formatLongDay } from '../../../format'
import NeedsYou from '../components/NeedsYou.vue'
import SummaryCards from '../components/SummaryCards.vue'
import UpcomingList from '../components/UpcomingList.vue'
import { useHojeStore } from '../store'

const store = useHojeStore()
const { today, loading, loadError } = storeToRefs(store)

onMounted(() => store.open())
</script>

<template>
  <section class="hoje">
    <h1>Hoje</h1>
    <p
      v-if="loadError"
      role="alert"
    >
      {{ loadError }}
    </p>
    <BaseButton
      v-if="loadError"
      @click="store.open()"
    >
      Tentar de novo
    </BaseButton>
    <p
      v-else-if="today === null && loading"
      class="muted"
    >
      Carregando o resumo de hoje…
    </p>

    <template v-if="today !== null">
      <p class="date">
        {{ formatLongDay(today.today) }}
      </p>
      <p
        v-if="today.truncated"
        role="status"
        class="notice"
      >
        Mostrando só os 500 pedidos ativos com entrega mais próxima. Os números e os avisos desta tela cobrem só esses pedidos.
      </p>
      <SummaryCards :counts="today.counts" />
      <NeedsYou :needs="today.needsYou" />
      <UpcomingList :orders="today.upcoming" />
      <RouterLink
        class="all"
        :to="{ name: 'orders' }"
      >
        Ver todos os pedidos
      </RouterLink>
    </template>
  </section>
</template>

<style scoped>
.hoje {
  display: flex;
  flex-direction: column;
  align-items: stretch;
  gap: 16px;
}
h1,
p {
  margin: 0;
}
.hoje > button {
  align-self: flex-start;
}
.date {
  color: var(--enc-ink-secondary);
}
.muted {
  color: var(--enc-ink-secondary);
}
[role='alert'] {
  font-weight: 600;
  color: var(--enc-status-alert-text);
}
.notice {
  padding: 8px 12px;
  border: 1px solid var(--enc-hairline);
  border-radius: var(--enc-radius-card);
  background: var(--enc-surface);
  font-weight: 600;
}
.all {
  align-self: flex-start;
  display: inline-flex;
  align-items: center;
  min-height: var(--enc-control-height);
  color: var(--enc-ink);
  font-weight: 600;
}
</style>
