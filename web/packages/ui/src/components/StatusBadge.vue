<script setup lang="ts">
/**
 * StatusBadge shows an order or item status as icon + word, the only way
 * status is ever shown. The word comes from the store's status workflow; the
 * color and icon come from the phase.
 */
import {
  IconBan,
  IconCircleCheck,
  IconClipboardList,
  IconPackageExport,
  IconTool,
  IconTruckDelivery,
} from '@tabler/icons-vue'
import { computed } from 'vue'

import { type Phase, type StatusTone, toneForPhase } from '../phase'

const props = defineProps<{
  /** pt-BR label from the order's status workflow. */
  label: string
  phase: Phase
}>()

const icons: Record<StatusTone, unknown> = {
  planned: IconClipboardList,
  waiting: IconTruckDelivery,
  assembly: IconTool,
  ready: IconCircleCheck,
  delivered: IconPackageExport,
  neutral: IconBan,
}

const tone = computed(() => toneForPhase(props.phase))
const icon = computed(() => icons[tone.value])
</script>

<template>
  <span
    class="enc-status-badge"
    :class="`enc-status-badge--${tone}`"
    :data-tone="tone"
  >
    <component
      :is="icon"
      :size="14"
      stroke-width="2"
      aria-hidden="true"
    />
    <span>{{ label }}</span>
  </span>
</template>

<style scoped>
.enc-status-badge {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 2px 6px;
  border-radius: var(--enc-radius-badge);
  font-size: 12px;
  font-weight: 600;
  line-height: 16px;
  white-space: nowrap;
}
.enc-status-badge--planned { background: var(--enc-status-planned-fill); color: var(--enc-status-planned-text); }
.enc-status-badge--waiting { background: var(--enc-status-waiting-fill); color: var(--enc-status-waiting-text); }
.enc-status-badge--assembly { background: var(--enc-status-assembly-fill); color: var(--enc-status-assembly-text); }
.enc-status-badge--ready { background: var(--enc-status-ready-fill); color: var(--enc-status-ready-text); }
.enc-status-badge--delivered { background: var(--enc-status-delivered-fill); color: var(--enc-status-delivered-text); }
.enc-status-badge--neutral {
  background: var(--enc-status-neutral-fill);
  color: var(--enc-status-neutral-text);
  box-shadow: inset 0 0 0 1px var(--enc-secondary-border);
}
</style>
