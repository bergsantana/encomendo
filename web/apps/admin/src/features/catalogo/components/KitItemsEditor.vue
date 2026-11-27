<script setup lang="ts">
/**
 * KitItemsEditor is "Itens do kit": the items a kit holds, each with its
 * quantity and "Cliente pode trocar". A swappable item offers "Pode trocar
 * por", with the items of its own category suggested first. Items are added
 * from a picker of the store's non-archived items; an item archived after the
 * kit was saved stays in its row, marked, until it is taken out.
 */
import type { Category, ItemSummary } from '@encomendo/api'
import { BaseButton, BaseInput, BaseSelect } from '@encomendo/ui'
import { computed, ref } from 'vue'

import type { KitItemDraft } from '../kitForm'
import ArchivedMark from './ArchivedMark.vue'

const props = defineProps<{
  /** catalogItems are every item of the store, archived ones too. */
  catalogItems: readonly ItemSummary[]
  categories: readonly Category[]
  /** canWrite shows the buttons that add and remove; without it the rows only show. */
  canWrite: boolean
  /** errors are the messages next to the fields, by key such as `items`, `items.0.quantity`. */
  errors: Readonly<Record<string, string>>
}>()

const rows = defineModel<KitItemDraft[]>({ required: true })

/** toAdd is the picker's choice, '' until one is made. */
const toAdd = ref('')

const itemsById = computed(() => new Map(props.catalogItems.map((item) => [item.id, item])))
const categoryNames = computed(() => new Map(props.categories.map((category) => [category.id, category.name])))
const pickerOptions = computed(() => {
  const inKit = new Set(rows.value.map((row) => row.itemId))
  const available = props.catalogItems.filter((item) => !item.archived && !inKit.has(item.id))
  return { available, options: [{ value: '', label: 'Escolha o item' }, ...available.map((item) => ({ value: item.id, label: item.name }))] }
})

interface AlternativeGroup {
  title: string | null
  items: ItemSummary[]
}

/**
 * alternativesFor lists what a row may be swapped for: the items of the same
 * category first, under "Sugestões da categoria {categoria}", then the rest.
 * An archived item stays listed only while the row still names it, so it can
 * be unticked.
 */
function alternativesFor(row: KitItemDraft): AlternativeGroup[] {
  const own = itemsById.value.get(row.itemId)
  const candidates = props.catalogItems.filter((item) => item.id !== row.itemId && (!item.archived || row.alternativeItemIds.includes(item.id)))
  // Without a category the store still knows (none, or one deleted meanwhile), there is nothing to suggest from.
  const ownCategory = own?.categoryId ?? null
  const categoryName = ownCategory === null ? undefined : categoryNames.value.get(ownCategory)
  const suggested = categoryName === undefined ? [] : candidates.filter((item) => item.categoryId === ownCategory)
  const others = candidates.filter((item) => !suggested.includes(item))
  const groups: AlternativeGroup[] = []
  if (suggested.length > 0) groups.push({ title: `Sugestões da categoria ${categoryName}`, items: suggested })
  if (others.length > 0) groups.push({ title: suggested.length > 0 ? 'Outros itens' : null, items: others })
  return groups
}

function patchRow(index: number, patch: Partial<KitItemDraft>): void {
  rows.value = rows.value.map((row, at) => (at === index ? { ...row, ...patch } : row))
}

function setSwappable(index: number, event: Event): void {
  if (event.target instanceof HTMLInputElement) patchRow(index, { swappable: event.target.checked })
}

function toggleAlternative(index: number, itemId: string, event: Event): void {
  const row = rows.value[index]
  if (row === undefined || !(event.target instanceof HTMLInputElement)) return
  const others = row.alternativeItemIds.filter((id) => id !== itemId)
  patchRow(index, { alternativeItemIds: event.target.checked ? [...others, itemId] : others })
}

function addItem(): void {
  const item = itemsById.value.get(toAdd.value)
  if (item === undefined) return
  rows.value = [...rows.value, { itemId: item.id, itemName: item.name, quantity: '1', swappable: false, alternativeItemIds: [], archived: false }]
  toAdd.value = ''
}

function removeRow(index: number): void {
  rows.value = rows.value.filter((_, at) => at !== index)
}

function alternativeLabel(item: ItemSummary): string {
  return item.archived ? `${item.name} (arquivado)` : item.name
}
</script>

<template>
  <fieldset class="items">
    <legend>Itens do kit</legend>
    <p
      v-if="errors.items"
      class="error"
      role="alert"
    >
      {{ errors.items }}
    </p>
    <fieldset
      v-for="(row, index) in rows"
      :key="row.itemId"
      class="row"
    >
      <legend>{{ row.itemName }}</legend>
      <ArchivedMark v-if="row.archived" />
      <BaseInput
        :model-value="row.quantity"
        label="Quantidade"
        inputmode="numeric"
        :error="errors[`items.${index}.quantity`] ?? null"
        @update:model-value="(quantity) => patchRow(index, { quantity })"
      />
      <label class="enc-check">
        <input
          type="checkbox"
          :checked="row.swappable"
          @change="(event) => setSwappable(index, event)"
        >
        Cliente pode trocar
      </label>
      <fieldset
        v-if="row.swappable"
        class="swap"
      >
        <legend>Pode trocar por</legend>
        <p
          v-if="errors[`items.${index}.alternatives`]"
          class="error"
          role="alert"
        >
          {{ errors[`items.${index}.alternatives`] }}
        </p>
        <component
          :is="group.title === null ? 'div' : 'fieldset'"
          v-for="group in alternativesFor(row)"
          :key="group.title ?? ''"
          class="choices"
        >
          <legend v-if="group.title !== null">
            {{ group.title }}
          </legend>
          <label
            v-for="item in group.items"
            :key="item.id"
            class="enc-check"
          >
            <input
              type="checkbox"
              :checked="row.alternativeItemIds.includes(item.id)"
              @change="(event) => toggleAlternative(index, item.id, event)"
            >
            {{ alternativeLabel(item) }}
          </label>
        </component>
      </fieldset>
      <div v-if="canWrite">
        <BaseButton
          :aria-label="`Remover ${row.itemName} do kit`"
          @click="removeRow(index)"
        >
          Remover do kit
        </BaseButton>
      </div>
    </fieldset>

    <div
      v-if="canWrite"
      class="picker"
    >
      <BaseSelect
        v-model="toAdd"
        label="Item a adicionar"
        :options="pickerOptions.options"
      />
      <BaseButton
        :disabled="toAdd === ''"
        @click="addItem"
      >
        Adicionar item
      </BaseButton>
    </div>
    <p
      v-if="canWrite && pickerOptions.available.length === 0"
      class="hint"
    >
      Nenhum item disponível para adicionar.
    </p>
  </fieldset>
</template>

<style scoped>
.items,
.row,
.swap,
.choices {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin: 0;
  padding: 12px;
  border: 1px solid var(--enc-hairline);
  border-radius: var(--enc-radius-card);
  min-width: 0;
}
.choices {
  max-height: 200px;
  overflow-y: auto;
}
div.choices {
  padding: 0;
  border: 0;
}
legend {
  padding: 0 4px;
  font-weight: 600;
}
.picker {
  display: flex;
  align-items: flex-end;
  gap: 8px;
}
.hint {
  margin: 0;
  color: var(--enc-ink-secondary);
}
.error {
  margin: 0;
  color: var(--enc-status-alert-text);
  font-weight: 600;
}

/* Phone: the item picker and its button stack, and the alternatives list grows with its checkboxes. */
@media (max-width: 640px) {
  .picker {
    flex-direction: column;
    align-items: stretch;
  }
  .choices {
    max-height: 60dvh;
  }
}
</style>
