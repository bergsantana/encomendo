<script setup lang="ts">
/**
 * CatalogTabs are the sections of Catálogo ("Itens", "Kits") as a tab list:
 * one tab is in the Tab order, and the arrow keys, Home and End move between
 * tabs, selecting the one they land on. Each tab controls a panel named
 * `panel-{id}` and is named `tab-{id}`; the view renders the panels.
 */
import { nextTick, useTemplateRef } from 'vue'

const props = defineProps<{
  tabs: readonly { id: string; label: string }[]
  /** label is the accessible name of the list. */
  label: string
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
    :aria-label="label"
  >
    <button
      v-for="(tab, index) in tabs"
      :id="`tab-${tab.id}`"
      :key="tab.id"
      type="button"
      role="tab"
      :aria-selected="selected === tab.id"
      :aria-controls="`panel-${tab.id}`"
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
  border-bottom: 1px solid var(--enc-hairline);
  width: 100%;
}
.tabs [role='tab'] {
  padding: 8px 14px;
  border: 0;
  background: none;
  color: var(--enc-ink);
  font: inherit;
  cursor: pointer;
}
.tabs [role='tab'][aria-selected='true'] {
  box-shadow: inset 0 -3px 0 var(--enc-accent);
  font-weight: 600;
}
</style>
