<script setup lang="ts">
/**
 * AlertBadge is "Pode atrasar" or "Atrasada": icon and word, in the alert
 * colors, shown on top of the order's status (never instead of it). The API
 * decides which alert an order has; this only draws it.
 */
import type { Alert } from '@encomendo/api'
import { IconAlertTriangle, IconClockExclamation } from '@encomendo/ui'

defineProps<{
  /** kind is `late` ("Atrasada") or `at_risk` ("Pode atrasar"). */
  kind: Alert['kind']
}>()
</script>

<template>
  <span
    class="alert-badge"
    :data-kind="kind"
  >
    <component
      :is="kind === 'late' ? IconClockExclamation : IconAlertTriangle"
      :size="14"
      stroke-width="2"
      aria-hidden="true"
    />
    <span>{{ kind === 'late' ? 'Atrasada' : 'Pode atrasar' }}</span>
  </span>
</template>

<style scoped>
.alert-badge {
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
  white-space: nowrap;
}
</style>
