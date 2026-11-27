<script setup lang="ts">
/**
 * AttributesDialog is "Campos dos itens": the store's item fields (such as
 * "Serve em") and their values. Items and kits pick values of these fields,
 * and the store page filters by them. Without `catalog:write` it only lists.
 */
import type { Attribute } from '@encomendo/api'
import { BaseButton, BaseDialog } from '@encomendo/ui'
import { ref } from 'vue'

import type { AttributeDraft } from '../taxonomy'
import AttributeCard from './AttributeCard.vue'

defineProps<{
  attributes: readonly Attribute[]
  canWrite: boolean
  /** activeId is the card the last save or deletion came from ('' for the new one); the errors belong to it. */
  activeId: string | null
  error: string | null
  fieldErrors: Readonly<Record<string, string>>
  saving: boolean
}>()

const emit = defineEmits<{
  close: []
  save: [attribute: Attribute | null, draft: AttributeDraft]
  remove: [attribute: Attribute]
  clearErrors: []
}>()

const adding = ref(false)

function add(): void {
  emit('clearErrors')
  adding.value = true
}

function discard(): void {
  adding.value = false
  emit('clearErrors')
}

defineExpose({
  /** saved closes the new-field card once the field exists. */
  saved(): void {
    adding.value = false
  },
})
</script>

<template>
  <BaseDialog
    title="Campos dos itens"
    @close="emit('close')"
  >
    <div class="body">
      <p class="hint">
        Campos como "Serve em" ajudam o cliente a achar o que serve para ele.
      </p>
      <AttributeCard
        v-for="attribute in attributes"
        :key="attribute.id"
        :attribute="attribute"
        :can-write="canWrite"
        :error="activeId === attribute.id ? error : null"
        :field-errors="activeId === attribute.id ? fieldErrors : {}"
        :saving="saving && activeId === attribute.id"
        @save="(draft) => emit('save', attribute, draft)"
        @remove="emit('remove', attribute)"
      />
      <AttributeCard
        v-if="adding"
        :attribute="null"
        :can-write="canWrite"
        :error="activeId === '' ? error : null"
        :field-errors="activeId === '' ? fieldErrors : {}"
        :saving="saving && activeId === ''"
        @save="(draft) => emit('save', null, draft)"
        @discard="discard"
      />
      <div class="buttons enc-sheet-actions">
        <BaseButton
          v-if="canWrite && !adding"
          @click="add"
        >
          Novo campo
        </BaseButton>
        <BaseButton @click="emit('close')">
          Fechar
        </BaseButton>
      </div>
    </div>
  </BaseDialog>
</template>

<style scoped>
.body {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.hint {
  margin: 0;
  color: var(--enc-ink-secondary);
}
.buttons {
  display: flex;
  gap: 8px;
}
</style>
