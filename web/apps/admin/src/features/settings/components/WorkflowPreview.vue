<script setup lang="ts">
/**
 * WorkflowPreview lists a preset's statuses in order: the staff name as a
 * badge in its phase's color, and under it what the client sees. Words come
 * from the preset; the color from the phase.
 */
import type { OrderStatus } from '@encomendo/api'
import { StatusBadge } from '@encomendo/ui'

defineProps<{
  statuses: readonly OrderStatus[]
}>()
</script>

<template>
  <ol class="steps">
    <li
      v-for="status in statuses"
      :key="status.key"
      class="step"
    >
      <StatusBadge
        :label="status.label"
        :phase="status.phase"
      />
      <span class="client">O cliente vê: {{ status.clientLabel }}</span>
    </li>
  </ol>
</template>

<style scoped>
.steps {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin: 0;
  padding: 0;
  list-style: none;
}
.step {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 2px;
}
.client {
  color: var(--enc-ink-secondary);
}
</style>
