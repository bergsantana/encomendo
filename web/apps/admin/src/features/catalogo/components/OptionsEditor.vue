<script setup lang="ts">
/**
 * OptionsEditor is "Opções do cliente": the choices a client makes when
 * ordering the item, each option with its own choices, extra price and extra
 * days. Prices and days are typed text here; the form turns them into cents
 * and numbers when it saves.
 */
import { BaseButton, BaseInput } from '@encomendo/ui'

import type { ChoiceDraft, OptionDraft } from '../itemForm'

defineProps<{
  /** canWrite shows the buttons that add and remove; without it the options only show. */
  canWrite: boolean
  /** errors are the messages next to the fields, by key such as `options.0.name`. */
  errors: Readonly<Record<string, string>>
}>()

const options = defineModel<OptionDraft[]>({ required: true })

function patchOption(index: number, patch: Partial<OptionDraft>): void {
  options.value = options.value.map((option, at) => (at === index ? { ...option, ...patch } : option))
}

function patchChoice(index: number, choiceIndex: number, patch: Partial<ChoiceDraft>): void {
  const option = options.value[index]
  if (option === undefined) return
  patchOption(index, { choices: option.choices.map((choice, at) => (at === choiceIndex ? { ...choice, ...patch } : choice)) })
}

function setRequired(index: number, event: Event): void {
  if (event.target instanceof HTMLInputElement) patchOption(index, { required: event.target.checked })
}

function addOption(): void {
  options.value = [...options.value, { name: '', required: false, choices: [{ label: '', extraPrice: '', extraDays: '' }] }]
}

function addChoice(index: number): void {
  const option = options.value[index]
  if (option !== undefined) patchOption(index, { choices: [...option.choices, { label: '', extraPrice: '', extraDays: '' }] })
}

function removeOption(index: number): void {
  options.value = options.value.filter((_, at) => at !== index)
}

function removeChoice(index: number, choiceIndex: number): void {
  const option = options.value[index]
  if (option !== undefined) patchOption(index, { choices: option.choices.filter((_, at) => at !== choiceIndex) })
}
</script>

<template>
  <fieldset class="options">
    <legend>Opções do cliente</legend>
    <p class="hint">
      O cliente escolhe uma destas opções ao pedir. Dias a mais entram no prazo.
    </p>
    <fieldset
      v-for="(option, index) in options"
      :key="option.id ?? `new-${index}`"
      class="option"
    >
      <legend>Opção {{ index + 1 }}</legend>
      <BaseInput
        :model-value="option.name"
        label="Nome da opção"
        :error="errors[`options.${index}.name`] ?? null"
        @update:model-value="(name) => patchOption(index, { name })"
      />
      <label class="check">
        <input
          type="checkbox"
          :checked="option.required"
          @change="(event) => setRequired(index, event)"
        >
        Obrigatória
      </label>
      <p
        v-if="errors[`options.${index}.choices`]"
        class="error"
        role="alert"
      >
        {{ errors[`options.${index}.choices`] }}
      </p>
      <fieldset
        v-for="(choice, at) in option.choices"
        :key="choice.id ?? `new-${at}`"
        class="choice"
      >
        <legend>Escolha {{ at + 1 }}</legend>
        <BaseInput
          :model-value="choice.label"
          label="Escolha"
          :error="errors[`options.${index}.choices.${at}.label`] ?? null"
          @update:model-value="(label) => patchChoice(index, at, { label })"
        />
        <BaseInput
          :model-value="choice.extraPrice"
          label="Acréscimo no preço"
          inputmode="decimal"
          :error="errors[`options.${index}.choices.${at}.extraPrice`] ?? null"
          @update:model-value="(extraPrice) => patchChoice(index, at, { extraPrice })"
        />
        <BaseInput
          :model-value="choice.extraDays"
          label="Dias a mais"
          inputmode="numeric"
          :error="errors[`options.${index}.choices.${at}.extraDays`] ?? null"
          @update:model-value="(extraDays) => patchChoice(index, at, { extraDays })"
        />
        <BaseButton
          v-if="canWrite"
          @click="removeChoice(index, at)"
        >
          Remover escolha
        </BaseButton>
      </fieldset>
      <div class="row">
        <BaseButton
          v-if="canWrite"
          @click="addChoice(index)"
        >
          Adicionar escolha
        </BaseButton>
        <BaseButton
          v-if="canWrite"
          @click="removeOption(index)"
        >
          Remover opção
        </BaseButton>
      </div>
    </fieldset>
    <div v-if="canWrite">
      <BaseButton @click="addOption">
        Nova opção
      </BaseButton>
    </div>
  </fieldset>
</template>

<style scoped>
.options,
.option,
.choice {
  display: flex;
  flex-direction: column;
  gap: 12px;
  margin: 0;
  padding: 12px;
  border: 1px solid var(--enc-hairline);
  border-radius: var(--enc-radius-card);
}
.choice {
  background: var(--enc-ground);
}
legend {
  padding: 0 4px;
  font-weight: 600;
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
.check {
  display: flex;
  align-items: center;
  gap: 6px;
}
.row {
  display: flex;
  gap: 8px;
}
</style>
