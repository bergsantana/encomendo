<script setup lang="ts">
/**
 * StatusTabs are the order list's tabs: "Ativos", one per status of the
 * store's list, "Entregues" and "Cancelados". They are a tab list: one tab is
 * in the Tab order and the arrow keys, Home and End move between them,
 * selecting the one they land on. The view renders the one panel.
 */
import { nextTick, useTemplateRef } from 'vue'

import { type OrderTab, tabElementId } from '../store'

const props = defineProps<{
  tabs: readonly OrderTab[]
}>()

/** selected is the id of the open tab. */
const selected = defineModel<string>({ required: true })

const list = useTemplateRef<HTMLElement>('list')

function targetFor(key: string, index: number): number | null {
  const count = props.tabs.length
  if (key === 'ArrowRight') return (index + 1) % count
  if (key === 'ArrowLeft') return (index - 1 + count) % count
  if (key === 'Home') return 0
  if (key === 'End') return count - 1
  return null
}

async function onKeydown(event: KeyboardEvent, index: number): Promise<void> {
  const target = targetFor(event.key, index)
  const tab = target === null ? undefined : props.tabs[target]
  if (target === null || tab === undefined) return
  event.preventDefault()
  selected.value = tab.id
  await nextTick()
  const button = list.value?.children[target]
  if (button instanceof HTMLElement) button.focus()
}
</script>

<template>
  <div
    ref="list"
    class="tabs"
    role="tablist"
    aria-label="Pedidos por status"
  >
    <button
      v-for="(tab, index) in tabs"
      :id="tabElementId(tab.id)"
      :key="tab.id"
      type="button"
      role="tab"
      aria-controls="orders-panel"
      :aria-selected="selected === tab.id"
      :tabindex="selected === tab.id ? 0 : -1"
      @click="selected = tab.id"
      @keydown="(event) => onKeydown(event, index)"
    >
      {{ tab.label }}
    </button>
  </div>
</template>

<style scoped>
.tabs {
  display: flex;
  gap: 4px;
  width: 100%;
  overflow-x: auto;
  border-bottom: 1px solid var(--enc-hairline);
}
.tabs [role='tab'] {
  flex: none;
  padding: 8px 14px;
  border: 0;
  background: none;
  color: var(--enc-ink);
  font: inherit;
  white-space: nowrap;
  cursor: pointer;
}
.tabs [role='tab'][aria-selected='true'] {
  box-shadow: inset 0 -3px 0 var(--enc-accent);
  font-weight: 600;
}

/* Phone: tall tabs that scroll sideways inside their row, so the page itself never does. */
@media (max-width: 640px) {
  .tabs [role='tab'] {
    min-height: var(--enc-control-height);
  }
}
</style>
