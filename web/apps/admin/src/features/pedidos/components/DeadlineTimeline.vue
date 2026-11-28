<script setup lang="ts">
/**
 * DeadlineTimeline is "Linha do tempo": one row per item on a band from
 * today to the delivery date, with the marks "Hoje", "Precisa chegar até",
 * "{Montagem}" and "Entrega". Each item says "Previsto: {data}" or "Recebido
 * em {data}", and an item with an alert is drawn in the alert color (the API
 * says which). The band is only a drawing; every date is written out as text
 * too, and on the phone the band is left out and the rows are a vertical list
 * of items with their dates.
 */
import type { Order } from '@encomendo/api'
import { computed } from 'vue'

import AlertBadge from '../../../components/AlertBadge.vue'
import { formatCivilDate } from '../../../format'
import { niche } from '../../../niche'
import { bandPosition, latestDay } from '../timeline'

const props = defineProps<{
  order: Order
  /** today is the store's today as "YYYY-MM-DD"; it only places the first mark. */
  today: string
}>()

/** end is the last day on the band: the delivery, or a later expected date so a late item stays on it. */
const end = computed(() => latestDay(props.order.items.map((item) => item.expectedDate), props.order.deliveryDate))

const marks = computed(() => [
  { key: 'today', position: 0 },
  { key: 'limit', position: bandPosition(props.order.arrivalLimit, props.today, end.value) },
  { key: 'delivery', position: bandPosition(props.order.deliveryDate, props.today, end.value) },
])

const assembly = computed(() => {
  const from = bandPosition(props.order.arrivalLimit, props.today, end.value)
  const to = bandPosition(props.order.deliveryDate, props.today, end.value)
  return { left: `${from}%`, width: `${Math.max(to - from, 0)}%` }
})

function barWidth(day: string): string {
  return `${bandPosition(day, props.today, end.value)}%`
}
</script>

<template>
  <section
    class="timeline"
    aria-labelledby="timeline-heading"
  >
    <h2 id="timeline-heading">
      Linha do tempo
    </h2>
    <ul class="legend">
      <li>Hoje: <span class="enc-number">{{ formatCivilDate(today) }}</span></li>
      <li>Precisa chegar até: <span class="enc-number">{{ formatCivilDate(order.arrivalLimit) }}</span></li>
      <li>{{ niche.montagem }}: <span class="enc-number">{{ formatCivilDate(order.arrivalLimit) }} a {{ formatCivilDate(order.deliveryDate) }}</span></li>
      <li>Entrega: <span class="enc-number">{{ formatCivilDate(order.deliveryDate) }}</span></li>
    </ul>

    <ul class="rows">
      <li
        v-for="item in order.items"
        :key="item.id"
        class="row"
        :data-alert="item.alerts.length > 0 ? 'true' : 'false'"
      >
        <span class="name">{{ item.name }}<AlertBadge
          v-if="item.alerts.length > 0"
          class="alert"
          :kind="item.alerts.includes('late') ? 'late' : 'at_risk'"
        /></span>
        <span class="band">
          <span
            class="assembly"
            :style="assembly"
          />
          <span
            v-for="mark in marks"
            :key="mark.key"
            class="mark"
            :style="{ left: `${mark.position}%` }"
          />
          <span
            class="bar"
            :style="{ width: barWidth(item.arrivedOn ?? item.expectedDate) }"
          />
        </span>
        <span class="date enc-number">
          {{ item.arrivedOn !== null ? `Recebido em ${formatCivilDate(item.arrivedOn)}` : `Previsto: ${formatCivilDate(item.expectedDate)}` }}
        </span>
      </li>
    </ul>
  </section>
</template>

<style scoped>
.timeline {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
h2 {
  margin: 0;
  font-size: 15px;
}
ul {
  margin: 0;
  padding: 0;
  list-style: none;
}
.legend {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 16px;
  color: var(--enc-ink-secondary);
}
.rows {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.row {
  display: grid;
  grid-template-columns: minmax(120px, 220px) 1fr minmax(120px, 160px);
  align-items: center;
  gap: 12px;
}
.band {
  position: relative;
  display: block;
  height: 16px;
  border-radius: 4px;
  background: var(--enc-ground);
}
.assembly {
  position: absolute;
  top: 0;
  bottom: 0;
  background: var(--enc-status-assembly-fill);
}
.mark {
  position: absolute;
  top: -2px;
  bottom: -2px;
  width: 2px;
  background: var(--enc-ink-secondary);
}
.bar {
  position: absolute;
  top: 4px;
  bottom: 4px;
  left: 0;
  border-radius: 4px;
  background: var(--enc-status-ready-text);
}
.row[data-alert='true'] .bar {
  background: var(--enc-status-alert-text);
}
.alert {
  margin-left: 8px;
}
.date {
  text-align: right;
}

/* Phone: no band; each item is a line with its name and its date. */
@media (max-width: 640px) {
  .row {
    grid-template-columns: 1fr;
    gap: 2px;
    padding: 8px 0;
    border-bottom: 1px solid var(--enc-hairline);
  }
  .band {
    display: none;
  }
  .name {
    font-weight: 600;
  }
  .date {
    text-align: left;
  }
}
</style>
