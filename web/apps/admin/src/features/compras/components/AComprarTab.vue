<script setup lang="ts">
/**
 * AComprarTab is Compras, "A comprar": one card per supplier, and after a
 * send "Pedido de compra C-0012 criado." with "Abrir o WhatsApp" (it stays
 * until the next action, in case the browser blocked the automatic opening),
 * "Copiar mensagem" and "Ver pedido de compra".
 */
import type { ToBuySupplier } from '@encomendo/api'
import { BaseButton } from '@encomendo/ui'
import { RouterLink } from 'vue-router'

import type { Preview, SentOrder } from '../store'
import SupplierCard from './SupplierCard.vue'

defineProps<{
  suppliers: readonly ToBuySupplier[]
  loaded: boolean
  loading: boolean
  error: string | null
  unchecked: ReadonlySet<string>
  preview: Preview | null
  sending: boolean
  sent: SentOrder | null
  copyStatus: string | null
  /** purchaseBeforeDeposit is "Liberar compras antes do sinal": it changes the hint of an empty list. */
  purchaseBeforeDeposit: boolean
}>()

const emit = defineEmits<{
  pick: [orderItemId: string, picked: boolean, supplierId: string]
  togglePreview: [supplierId: string]
  send: [supplierId: string]
  copy: [text: string]
  openLink: []
  retry: []
}>()
</script>

<template>
  <div class="tab">
    <p
      v-if="error"
      role="alert"
    >
      {{ error }}
    </p>
    <BaseButton
      v-if="error && !loaded"
      @click="emit('retry')"
    >
      Tentar de novo
    </BaseButton>

    <div
      v-if="sent"
      class="sent"
    >
      <p role="status">
        Pedido de compra <span class="enc-code">{{ sent.number }}</span> criado.
      </p>
      <div class="sent__actions">
        <BaseButton @click="emit('openLink')">
          Abrir o WhatsApp
        </BaseButton>
        <BaseButton @click="emit('copy', sent.text)">
          Copiar mensagem
        </BaseButton>
        <RouterLink
          class="link-button"
          :to="{ name: 'supplier-order', params: { supplierOrderId: sent.supplierOrderId } }"
        >
          Ver pedido de compra
        </RouterLink>
      </div>
    </div>
    <p
      v-if="copyStatus"
      role="status"
    >
      {{ copyStatus }}
    </p>

    <SupplierCard
      v-for="supplier in suppliers"
      :key="supplier.supplierId"
      :supplier="supplier"
      :unchecked="unchecked"
      :preview="preview?.supplierId === supplier.supplierId ? preview : null"
      :sending="sending"
      @pick="(orderItemId, picked) => emit('pick', orderItemId, picked, supplier.supplierId)"
      @toggle-preview="emit('togglePreview', supplier.supplierId)"
      @send="emit('send', supplier.supplierId)"
      @copy="(text) => emit('copy', text)"
    />

    <p
      v-if="suppliers.length === 0 && !loaded && !error"
      class="muted"
    >
      Carregando as compras…
    </p>
    <div
      v-else-if="suppliers.length === 0 && loaded"
      class="empty"
    >
      <p><strong>Nada para comprar agora.</strong></p>
      <p class="muted">
        {{ purchaseBeforeDeposit ? 'Os itens aparecem aqui quando o pedido sai do rascunho.' : 'Os itens aparecem aqui quando o cliente paga o sinal.' }}
      </p>
    </div>
  </div>
</template>

<style scoped>
.tab {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 16px;
  width: 100%;
}
p {
  margin: 0;
}
.muted {
  color: var(--enc-ink-secondary);
}
[role='alert'] {
  color: var(--enc-status-alert-text);
  font-weight: 600;
}
[role='status'] {
  font-weight: 600;
}
.sent {
  display: flex;
  flex-direction: column;
  gap: 8px;
  width: 100%;
  box-sizing: border-box;
  padding: 12px 16px;
  border: 1px solid var(--enc-hairline);
  border-radius: var(--enc-radius-card);
  background: var(--enc-surface);
}
.sent__actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.link-button {
  display: inline-flex;
  align-items: center;
  height: var(--enc-control-height, 32px);
  padding: 0 14px;
  border: 1px solid var(--enc-secondary-border);
  border-radius: var(--enc-radius-card);
  background: var(--enc-surface);
  color: var(--enc-ink);
  font-weight: 600;
}

/* Phone: the three actions one under the other, full width. */
@media (max-width: 640px) {
  .sent__actions > * {
    flex: 1 1 100%;
    justify-content: center;
  }
}
</style>
