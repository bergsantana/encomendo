<script setup lang="ts">
/**
 * SuspendedBadge marks a store the platform suspended, as icon + word like
 * every status (ADR 0022). It uses the neutral status tokens: a suspended
 * store is neither late nor ready, so no alert color applies.
 */
import { IconLock } from '@tabler/icons-vue'
</script>

<template>
  <span class="enc-suspended-badge">
    <IconLock
      :size="14"
      stroke-width="2"
      aria-hidden="true"
    />
    <span>Suspensa</span>
  </span>
</template>

<style scoped>
.enc-suspended-badge {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 2px 6px;
  border-radius: var(--enc-radius-badge);
  background: var(--enc-status-neutral-fill);
  color: var(--enc-status-neutral-text);
  box-shadow: inset 0 0 0 1px var(--enc-secondary-border);
  font-size: 12px;
  font-weight: 600;
  line-height: 16px;
  white-space: nowrap;
}
</style>
