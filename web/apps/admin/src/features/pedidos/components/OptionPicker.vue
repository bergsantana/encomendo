<script setup lang="ts">
/**
 * OptionPicker is the client's options of one item ("Cor", "Tamanho"): one
 * choice each, with its extra price and extra days as the catalog has them.
 * A required option asks "Escolha uma opção" until chosen; the form refuses
 * to save without it ("Escolha {opção}."). Prices and days of the order come
 * from the quote, never from adding these up.
 */
import type { Item } from '@encomendo/api'
import { BaseSelect } from '@encomendo/ui'

import { formatDays, formatMoney } from '../../../format'
import { optionKey } from '../orderForm'

defineProps<{
  itemId: string
  options: Item['options']
  /** choices maps `{itemId}:{optionId}` to the chosen choice id. */
  choices: Readonly<Record<string, string>>
  /** errors are the messages by form field, `option.{line}.{item}.{option}`. */
  errors: Readonly<Record<string, string>>
  lineKey: string
}>()

const emit = defineEmits<{
  choose: [option: Item['options'][number], choiceId: string]
}>()

function choiceLabel(choice: Item['options'][number]['choices'][number]): string {
  const extras: string[] = []
  if (choice.extraPrice.cents > 0) extras.push(`+ ${formatMoney(choice.extraPrice.cents, choice.extraPrice.currency)}`)
  if (choice.extraDays > 0) extras.push(`+ ${formatDays(choice.extraDays)}`)
  return extras.length === 0 ? choice.label : `${choice.label} (${extras.join(', ')})`
}
</script>

<template>
  <div class="options">
    <BaseSelect
      v-for="option in options"
      :key="option.id"
      :model-value="choices[optionKey(itemId, option.id)] ?? ''"
      :label="option.name"
      :options="[
        { value: '', label: option.required ? 'Escolha uma opção' : 'Nenhuma' },
        ...option.choices.map((choice) => ({ value: choice.id, label: choiceLabel(choice) })),
      ]"
      :error="errors[`option.${lineKey}.${itemId}.${option.id}`] ?? null"
      @update:model-value="emit('choose', option, $event)"
    />
  </div>
</template>

<style scoped>
.options {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
}

@media (max-width: 640px) {
  .options {
    flex-direction: column;
  }
}
</style>
