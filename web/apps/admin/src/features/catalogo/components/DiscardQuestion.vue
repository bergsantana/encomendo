<script setup lang="ts">
/**
 * DiscardQuestion is "Descartar as alterações?", asked when a form with edits
 * not saved is closed. The keyboard lands on the safe answer, "Continuar
 * editando", and the question is read out with it.
 */
import { BaseButton } from '@encomendo/ui'
import { onMounted, useTemplateRef } from 'vue'

const emit = defineEmits<{
  /** keep goes back to the form. */
  keep: []
  /** discard closes the form and drops the edits. */
  discard: []
}>()

const question = useTemplateRef<HTMLElement>('question')

onMounted(() => question.value?.querySelector('button')?.focus())
</script>

<template>
  <div
    ref="question"
    class="discard"
    role="group"
    aria-label="Descartar as alterações"
  >
    <p>Descartar as alterações?</p>
    <BaseButton @click="emit('keep')">
      Continuar editando
    </BaseButton>
    <BaseButton @click="emit('discard')">
      Descartar
    </BaseButton>
  </div>
</template>

<style scoped>
.discard {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px;
  background: var(--enc-ground);
  border-radius: var(--enc-radius-card);
}
.discard p {
  flex: 1;
  margin: 0;
  font-weight: 600;
}
</style>
