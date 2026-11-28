<script setup lang="ts">
/**
 * AlertTag is "Pode atrasar" or "Atrasada" as icon + word, in the alert
 * colors of the design system. It sits on top of the status, never in its
 * place. The Pedidos screens have the same tag; the two meet when the
 * branches merge.
 */
import type { Alert } from '@encomendo/api'

defineProps<{
  kind: Alert['kind']
}>()
</script>

<template>
  <span
    class="tag"
    :data-kind="kind"
  >
    <!-- Tabler outline "alert-triangle" (at risk) and "alert-circle" (late). -->
    <svg
      class="icon"
      viewBox="0 0 24 24"
      width="14"
      height="14"
      fill="none"
      stroke="currentColor"
      stroke-width="2"
      stroke-linecap="round"
      stroke-linejoin="round"
      aria-hidden="true"
    >
      <template v-if="kind === 'late'">
        <path d="M3 12a9 9 0 1 0 18 0a9 9 0 0 0 -18 0" />
        <path d="M12 8v4" />
        <path d="M12 16h.01" />
      </template>
      <template v-else>
        <path d="M12 9v4" />
        <path d="M10.363 3.591l-8.106 13.534a1.914 1.914 0 0 0 1.636 2.871h16.214a1.914 1.914 0 0 0 1.636 -2.87l-8.106 -13.536a1.914 1.914 0 0 0 -3.274 0z" />
        <path d="M12 16h.01" />
      </template>
    </svg>
    <span>{{ kind === 'late' ? 'Atrasada' : 'Pode atrasar' }}</span>
  </span>
</template>

<style scoped>
.tag {
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
