<script setup lang="ts">
/**
 * AttributeCard is one item field ("Serve em") with its values, edited as a
 * draft until "Salvar campo". A value that items or kits use, or a field with
 * such values, asks before it goes: removing it takes it off all of them
 * (ADR 0025).
 */
import type { Attribute } from '@encomendo/api'
import { BaseButton, BaseInput } from '@encomendo/ui'
import { computed, ref, watch } from 'vue'

import type { AttributeDraft } from '../taxonomy'

const props = defineProps<{
  /** attribute is the stored field, or null for a field not saved yet. */
  attribute: Attribute | null
  canWrite: boolean
  /** error and fieldErrors are the last save or deletion of this card; null and empty when it was another one. */
  error: string | null
  fieldErrors: Readonly<Record<string, string>>
  saving?: boolean
}>()

const emit = defineEmits<{
  save: [draft: AttributeDraft]
  remove: []
  discard: []
}>()

function fromAttribute(attribute: Attribute | null): AttributeDraft {
  return {
    name: attribute?.name ?? '',
    values: attribute?.values.map((value) => ({ id: value.id, label: value.label })) ?? [{ label: '' }],
  }
}

const draft = ref<AttributeDraft>(fromAttribute(props.attribute))
/** asking is what a confirmation is about: one value, or the whole field. */
const asking = ref<{ kind: 'value'; index: number } | { kind: 'field' } | null>(null)

// A saved or re-read field replaces the draft.
watch(
  () => props.attribute,
  (attribute) => {
    draft.value = fromAttribute(attribute)
    asking.value = null
  },
)

const usage = computed(() => new Map(props.attribute?.values.map((value) => [value.id, value.usageCount]) ?? []))
const fieldUsage = computed(() => [...usage.value.values()].reduce((sum, count) => sum + count, 0))

const question = computed(() => {
  const ask = asking.value
  if (ask === null) return ''
  if (ask.kind === 'field') return `Remover ${draft.value.name}? Seus valores saem de até ${fieldUsage.value} itens e kits.`
  const value = draft.value.values[ask.index]
  return `Remover ${value?.label ?? ''}? Ele sai de ${usageOf(ask.index)} itens e kits.`
})

function usageOf(index: number): number {
  const id = draft.value.values[index]?.id
  return id === undefined ? 0 : (usage.value.get(id) ?? 0)
}

function setName(name: string): void {
  draft.value = { ...draft.value, name }
}

function setValue(index: number, label: string): void {
  draft.value = { ...draft.value, values: draft.value.values.map((value, at) => (at === index ? { ...value, label } : value)) }
}

function addValue(): void {
  draft.value = { ...draft.value, values: [...draft.value.values, { label: '' }] }
}

function dropValue(index: number): void {
  draft.value = { ...draft.value, values: draft.value.values.filter((_, at) => at !== index) }
}

function removeValue(index: number): void {
  if (usageOf(index) > 0) asking.value = { kind: 'value', index }
  else dropValue(index)
}

function removeField(): void {
  if (props.attribute === null) emit('discard')
  else if (fieldUsage.value > 0) asking.value = { kind: 'field' }
  else emit('remove')
}

function confirm(): void {
  const ask = asking.value
  asking.value = null
  if (ask === null) return
  if (ask.kind === 'field') emit('remove')
  else dropValue(ask.index)
}
</script>

<template>
  <form
    class="card"
    @submit.prevent="emit('save', draft)"
  >
    <p
      v-if="error"
      role="alert"
      class="error"
    >
      {{ error }}
    </p>
    <fieldset
      class="fields"
      :disabled="!canWrite"
    >
      <BaseInput
        :model-value="draft.name"
        label="Nome do campo"
        :error="fieldErrors.name ?? null"
        @update:model-value="setName"
      />
      <fieldset class="values">
        <legend>Valores</legend>
        <div
          v-for="(value, index) in draft.values"
          :key="value.id ?? `new-${index}`"
          class="value"
        >
          <BaseInput
            :model-value="value.label"
            :label="`Valor ${index + 1}`"
            :error="fieldErrors[`values.${index}`] ?? null"
            @update:model-value="(label) => setValue(index, label)"
          />
          <BaseButton
            v-if="canWrite"
            :aria-label="`Remover valor ${value.label || index + 1}`"
            @click="removeValue(index)"
          >
            Remover valor
          </BaseButton>
        </div>
        <div v-if="canWrite">
          <BaseButton @click="addValue">
            Adicionar valor
          </BaseButton>
        </div>
      </fieldset>
    </fieldset>
    <div
      v-if="asking"
      class="ask"
      role="group"
      aria-label="Confirmar remoção"
    >
      <p>{{ question }}</p>
      <BaseButton @click="asking = null">
        Cancelar
      </BaseButton>
      <BaseButton
        variant="primary"
        @click="confirm"
      >
        Remover
      </BaseButton>
    </div>
    <div
      v-if="canWrite"
      class="buttons"
    >
      <BaseButton
        type="submit"
        :disabled="saving"
      >
        Salvar campo
      </BaseButton>
      <BaseButton
        :aria-label="`Excluir campo ${draft.name}`.trim()"
        @click="removeField"
      >
        Excluir campo
      </BaseButton>
    </div>
  </form>
</template>

<style scoped>
.card {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 12px;
  border: 1px solid var(--enc-hairline);
  border-radius: var(--enc-radius-card);
}
.fields,
.values {
  display: flex;
  flex-direction: column;
  gap: 12px;
  min-width: 0;
  margin: 0;
  padding: 0;
  border: 0;
}
legend {
  padding: 0;
  font-weight: 600;
}
.value {
  display: flex;
  align-items: flex-end;
  gap: 8px;
}
.ask {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px;
  background: var(--enc-ground);
  border-radius: var(--enc-radius-card);
}
.ask p {
  flex: 1;
  margin: 0;
  font-weight: 600;
}
.buttons {
  display: flex;
  gap: 8px;
}
.error {
  margin: 0;
  color: var(--enc-status-alert-text);
  font-weight: 600;
}
</style>
