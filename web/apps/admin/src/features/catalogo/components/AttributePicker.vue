<script setup lang="ts">
/**
 * AttributePicker is "Campos" on an item or a kit: one group per store field
 * (such as "Serve em"), with a box per value. Several values may be picked.
 * A field with no values is not offered.
 */
import type { Attribute } from '@encomendo/api'
import { computed } from 'vue'

const props = defineProps<{
  attributes: readonly Attribute[]
}>()

/** picked holds the ids of the chosen values. */
const picked = defineModel<string[]>({ required: true })

const pickable = computed(() => props.attributes.filter((attribute) => attribute.values.length > 0))

function pick(valueId: string, event: Event): void {
  if (!(event.target instanceof HTMLInputElement)) return
  const others = picked.value.filter((id) => id !== valueId)
  picked.value = event.target.checked ? [...others, valueId] : others
}
</script>

<template>
  <fieldset
    v-for="attribute in pickable"
    :key="attribute.id"
    class="attribute"
  >
    <legend>{{ attribute.name }}</legend>
    <label
      v-for="value in attribute.values"
      :key="value.id"
      class="enc-check"
    >
      <input
        type="checkbox"
        :checked="picked.includes(value.id)"
        @change="(event) => pick(value.id, event)"
      >
      {{ value.label }}
    </label>
  </fieldset>
</template>

<style scoped>
.attribute {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin: 0;
  padding: 12px;
  border: 1px solid var(--enc-hairline);
  border-radius: var(--enc-radius-card);
}
legend {
  padding: 0 4px;
  font-weight: 600;
}
</style>
