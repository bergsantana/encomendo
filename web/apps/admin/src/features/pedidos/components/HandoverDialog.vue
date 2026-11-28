<script setup lang="ts">
/**
 * HandoverDialog asks "Como o pedido foi entregue?" before the move to the
 * status that hands the order over: "Entrega" or "Retirada na loja". It
 * starts on how the order was planned.
 */
import { BaseButton, BaseDialog } from '@encomendo/ui'
import { ref } from 'vue'

import type { Handover } from '../labels'

const props = defineProps<{
  /** status is the label of the status the order moves to. */
  status: string
  planned: Handover
  busy: boolean
  /** error is why the move was refused (a guard, a conflict), shown here because the sheet covers the page. */
  error: string | null
}>()

const emit = defineEmits<{
  confirm: [handover: Handover]
  close: []
}>()

const chosen = ref<Handover>(props.planned)
</script>

<template>
  <BaseDialog
    title="Como o pedido foi entregue?"
    @close="emit('close')"
  >
    <form
      class="form"
      @submit.prevent="emit('confirm', chosen)"
    >
      <p
        v-if="error"
        role="alert"
        class="error"
      >
        {{ error }}
      </p>
      <fieldset>
        <legend class="enc-visually-hidden">
          Como o pedido foi entregue?
        </legend>
        <label class="enc-check">
          <input
            v-model="chosen"
            type="radio"
            name="how"
            value="delivery"
          >
          Entrega
        </label>
        <label class="enc-check">
          <input
            v-model="chosen"
            type="radio"
            name="how"
            value="pickup"
          >
          Retirada na loja
        </label>
      </fieldset>
      <div class="buttons enc-sheet-actions">
        <BaseButton
          variant="primary"
          type="submit"
          :disabled="busy"
        >
          Mover para {{ status }}
        </BaseButton>
        <BaseButton @click="emit('close')">
          Voltar
        </BaseButton>
      </div>
    </form>
  </BaseDialog>
</template>

<style scoped>
.form {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
fieldset {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin: 0;
  padding: 0;
  border: 0;
}
.error {
  margin: 0;
  color: var(--enc-status-alert-text);
  font-weight: 600;
}
.buttons {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
</style>
