<script setup lang="ts">
/**
 * LineEditor is one line of the order form: a kit with its items (a
 * swappable one offers "Trocar" with the kit's alternatives, and a swap reads
 * "{item} no lugar de {item}") or a loose item. Each line has "Quantidade",
 * the client's options and "Remover". The line's price and its items' dates
 * are what the quote answered; nothing is added up here.
 */
import type { Item, Kit, Quote } from '@encomendo/api'
import { BaseButton, BaseInput, BaseSelect } from '@encomendo/ui'

import { formatCivilDate, formatMoney } from '../../../format'
import type { LineDraft } from '../orderForm'
import OptionPicker from './OptionPicker.vue'

const props = defineProps<{
  line: LineDraft
  /** kit is the kit of a kit line, read with its items. */
  kit?: Kit
  /** items are the catalog items read for the form, by id. */
  items: Readonly<Record<string, Item>>
  /** quoted is what the quote said of this line, once it has answered. */
  quoted?: Quote['lines'][number]
  errors: Readonly<Record<string, string>>
  /** archivedNames are the items or kits the last answer refused as archived. */
  archivedNames: readonly string[]
}>()

const emit = defineEmits<{
  quantity: [text: string]
  swap: [itemId: string, withItemId: string]
  choose: [itemId: string, option: Item['options'][number], choiceId: string]
  remove: []
}>()

interface Slot {
  /** original is the kit's own item for this place; `current` is what the order holds now. */
  original: string
  current: string
  alternatives: string[]
  swappable: boolean
}

function slots(): Slot[] {
  if (props.line.kind === 'item') return [{ original: props.line.catalogId, current: props.line.catalogId, alternatives: [], swappable: false }]
  return (props.kit?.items ?? []).map((item) => ({
    original: item.itemId,
    current: props.line.swaps[item.itemId] ?? item.itemId,
    alternatives: item.swappable ? item.alternativeItemIds : [],
    swappable: item.swappable && item.alternativeItemIds.length > 0,
  }))
}

function nameOf(itemId: string): string {
  return props.items[itemId]?.name ?? props.kit?.items.find((item) => item.itemId === itemId)?.itemName ?? ''
}

function swapOptions(slot: Slot): { value: string; label: string }[] {
  return [slot.original, ...slot.alternatives].map((id) => ({ value: id, label: nameOf(id) }))
}

const refused = (): boolean => props.archivedNames.includes(props.line.name)
</script>

<template>
  <li class="line">
    <div class="head">
      <h3>{{ line.name }}</h3>
      <span
        v-if="quoted"
        class="enc-number price"
      >{{ formatMoney(quoted.price.cents, quoted.price.currency) }}</span>
    </div>
    <p
      v-if="refused()"
      class="error"
      role="alert"
    >
      {{ line.name }} está arquivado e não pode entrar no pedido.
    </p>

    <div class="quantity">
      <BaseInput
        :model-value="line.quantity"
        label="Quantidade"
        inputmode="numeric"
        :error="errors[`quantity.${line.key}`] ?? null"
        @update:model-value="emit('quantity', $event)"
      />
    </div>

    <ul class="slots">
      <li
        v-for="slot in slots()"
        :key="slot.original"
        class="slot"
      >
        <fieldset>
          <legend>{{ nameOf(slot.current) }}</legend>
          <template v-if="slot.swappable">
            <BaseSelect
              :model-value="slot.current"
              :label="`Trocar ${nameOf(slot.original)}`"
              :options="swapOptions(slot)"
              @update:model-value="emit('swap', slot.original, $event)"
            />
            <p
              v-if="slot.current !== slot.original"
              class="swapped"
            >
              {{ nameOf(slot.current) }} no lugar de {{ nameOf(slot.original) }}
            </p>
          </template>
          <OptionPicker
            :item-id="slot.current"
            :options="items[slot.current]?.options ?? []"
            :choices="line.choices"
            :errors="errors"
            :line-key="line.key"
            @choose="(option, choiceId) => emit('choose', slot.current, option, choiceId)"
          />
        </fieldset>
      </li>
    </ul>

    <ul
      v-if="quoted"
      class="dates"
    >
      <li
        v-for="item in quoted.items"
        :key="item.catalogItemId"
      >
        {{ item.name }}: previsto para {{ formatCivilDate(item.expectedDate) }}
      </li>
    </ul>

    <div>
      <BaseButton
        :aria-label="`Remover ${line.name}`"
        @click="emit('remove')"
      >
        Remover
      </BaseButton>
    </div>
  </li>
</template>

<style scoped>
.line {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 12px;
  border: 1px solid var(--enc-hairline);
  border-radius: var(--enc-radius-card);
  background: var(--enc-surface);
  list-style: none;
}
.head {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  justify-content: space-between;
  gap: 4px 12px;
}
h3,
p {
  margin: 0;
}
h3 {
  font-size: 15px;
}
.quantity {
  max-width: 160px;
}
.slots,
.dates {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin: 0;
  padding: 0;
  list-style: none;
}
.dates {
  color: var(--enc-ink-secondary);
}
fieldset {
  display: flex;
  flex-direction: column;
  gap: 8px;
  min-width: 0;
  margin: 0;
  padding: 0;
  border: 0;
}
legend {
  padding: 0;
  font-weight: 600;
}
.swapped {
  color: var(--enc-ink-secondary);
}
.error {
  color: var(--enc-status-alert-text);
  font-weight: 600;
}

@media (max-width: 640px) {
  .quantity {
    max-width: none;
  }
}
</style>
