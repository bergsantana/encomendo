<script setup lang="ts">
/**
 * ReleaseDialog confirms "Liberar sem sinal" (owner): the order goes ahead
 * before the deposit is paid, so purchases and production may start.
 */
import { BaseButton, BaseDialog } from '@encomendo/ui'

defineProps<{
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
    title="Liberar o pedido sem o sinal?"
    @close="emit('close')"
  >
    <div class="form">
      <p
        v-if="error"
        role="alert"
      >
        {{ error }}
      </p>
      <p>Liberar o pedido sem o sinal? As compras e a produção podem começar antes do pagamento.</p>
      <div class="buttons enc-sheet-actions">
        <BaseButton
          variant="primary"
          :disabled="busy"
          @click="emit('confirm')"
        >
          Liberar pedido
        </BaseButton>
        <BaseButton @click="emit('close')">
          Voltar
        </BaseButton>
      </div>
    </div>
  </BaseDialog>
</template>

<style scoped>
.form {
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
