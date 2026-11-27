<script setup lang="ts">
/** ArchivedMark is the "Arquivado" tag beside the name of an archived item or kit: icon and word, never color alone. */
import { IconArchive } from '@encomendo/ui'
</script>

<template>
  <span class="archived">
    <IconArchive
      :size="14"
      aria-hidden="true"
    />
    Arquivado
  </span>
</template>

<style scoped>
.archived {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  margin-left: 8px;
  padding: 0 6px;
  border-radius: var(--enc-radius-badge);
  background: var(--enc-status-neutral-fill);
  color: var(--enc-status-neutral-text);
}
</style>
