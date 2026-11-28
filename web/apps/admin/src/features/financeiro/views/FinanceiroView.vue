<script setup lang="ts">
/**
 * FinanceiroView is the owner's money view (`finance:read`, the Dono only):
 * the period, what was received in it, what is still to receive or give
 * back, and the margin of the orders handed over. Every figure is the
 * server's; the route and the menu hide the screen from other roles, and the
 * API refuses them again.
 */
import { BaseButton } from '@encomendo/ui'
import { storeToRefs } from 'pinia'
import { onMounted } from 'vue'

import MarginsTable from '../components/MarginsTable.vue'
import PeriodPicker from '../components/PeriodPicker.vue'
import ReceivedPanel from '../components/ReceivedPanel.vue'
import ToReceivePanel from '../components/ToReceivePanel.vue'
import { useFinanceStore } from '../store'

const store = useFinanceStore()
const { summary, preset, from, to, loading, error } = storeToRefs(store)

onMounted(() => store.open())
</script>

<template>
  <section class="financeiro">
    <h1>Financeiro</h1>
    <PeriodPicker
      :preset="preset"
      :from="from"
      :to="to"
      :error="error"
      @choose="store.choose($event)"
      @days="(start, end) => store.setDays(start, end)"
    />
    <BaseButton
      v-if="error"
      @click="store.retry()"
    >
      Tentar de novo
    </BaseButton>
    <p
      v-else-if="summary === null && loading"
      class="muted"
    >
      Carregando o financeiro…
    </p>

    <template v-if="summary !== null">
      <p
        v-if="summary.truncated"
        role="status"
        class="notice"
      >
        A loja tem mais pedidos do que o Financeiro lê de uma vez. "A receber", "A devolver" e as margens cobrem só parte dos pedidos.
      </p>
      <ReceivedPanel :received="summary.received" />
      <ToReceivePanel
        :to-receive="summary.toReceive"
        :to-refund="summary.toRefund"
      />
      <MarginsTable
        :margins="summary.margins"
        :totals="summary.marginsTotal"
      />
    </template>
  </section>
</template>

<style scoped>
.financeiro {
  display: flex;
  flex-direction: column;
  align-items: stretch;
  gap: 16px;
}
.financeiro > button {
  align-self: flex-start;
}
h1,
p {
  margin: 0;
}
.muted {
  color: var(--enc-ink-secondary);
}
.notice {
  padding: 8px 12px;
  border: 1px solid var(--enc-hairline);
  border-radius: var(--enc-radius-card);
  background: var(--enc-surface);
  font-weight: 600;
}
</style>
