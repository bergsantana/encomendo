<script setup lang="ts">
/**
 * UndoDialog asks before "Desfazer envio": the items go back to "A comprar"
 * and the C-number is never used again. If something was received meanwhile,
 * the API refuses and its message shows above the question. A sheet on the
 * phone, with "Voltar" in the sticky footer.
 */
import { BaseButton, BaseDialog } from '@encomendo/ui'

defineProps<{
  /** number is the pedido de compra's, "C-0012". */
  number: string
  error: string | null
  busy: boolean
}>()

const emit = defineEmits<{
  confirm: []
  close: []
}>()
</script>

<template>
  <BaseDialog
    title="Desfazer envio"
    @close="emit('close')"
  >
    <div class="body">
      <p
        v-if="error"
        role="alert"
      >
        {{ error }}
      </p>
      <p>Desfazer o envio do pedido de compra {{ number }}? Os itens voltam para A comprar. O número {{ number }} não será usado de novo.</p>
      <div class="buttons enc-sheet-actions">
        <BaseButton
          variant="primary"
          :disabled="busy"
          @click="emit('confirm')"
        >
          Desfazer envio
        </BaseButton>
        <BaseButton @click="emit('close')">
          Voltar
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
p {
  margin: 0;
}
[role='alert'] {
  color: var(--enc-status-alert-text);
  font-weight: 600;
}
.buttons {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
</style>
