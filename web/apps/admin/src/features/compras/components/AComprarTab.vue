<script setup lang="ts">
/**
 * AComprarTab is Compras, "A comprar": one card per supplier, and after a
 * send "Pedido de compra C-0012 criado." with "Abrir o WhatsApp" (it stays
 * until the next action, in case the browser blocked the automatic opening),
 * "Copiar mensagem" and "Ver pedido de compra".
 */
import type { ToBuySupplier } from '@encomendo/api'
import { BaseButton } from '@encomendo/ui'
import { computed } from 'vue'
import { RouterLink } from 'vue-router'

import type { CardError, CopyStatus, Preview, SentOrder } from '../store'
import SupplierCard from './SupplierCard.vue'

const props = defineProps<{
  suppliers: readonly ToBuySupplier[]
  loaded: boolean
  loading: boolean
  error: string | null
  unchecked: ReadonlySet<string>
  preview: Preview | null
  sending: boolean
  sent: SentOrder | null
  copyStatus: CopyStatus | null
  /** cardError is the refusal of a send, shown on its supplier's card. */
  cardError: CardError | null
  /** truncated says the API read only part of the store's orders: the list may not have everything. */
  truncated: boolean
  /** purchaseBeforeDeposit is "Liberar compras antes do sinal": it changes the hint of an empty list. */
  purchaseBeforeDeposit: boolean
}>()

const emit = defineEmits<{
  pick: [orderItemId: string, picked: boolean, supplierId: string]
  togglePreview: [supplierId: string]
  send: [supplierId: string]
  copy: [text: string, supplierId: string | null]
  openLink: []
  retry: []
}>()

/** A refusal for a supplier whose card is gone (every item of it left the list) has no card to sit on: it shows at the top. */
const orphanError = computed(() => {
  const refused = props.cardError
  if (refused === null || props.suppliers.some((supplier) => supplier.supplierId === refused.supplierId)) return null
  return refused.message
})

function forCard(supplierId: string): string | null {
  return props.cardError?.supplierId === supplierId ? props.cardError.message : null
}

function statusFor(supplierId: string): string | null {
  return props.copyStatus?.supplierId === supplierId ? props.copyStatus.text : null
}
</script>

<template>
  <div class="tab">
    <p
      v-if="error ?? orphanError"
      role="alert"
    >
      {{ error ?? orphanError }}
    </p>
    <p
      v-if="truncated"
      role="status"
    >
      Mostrando só os itens dos 500 pedidos ativos com entrega mais próxima. A lista pode não ter tudo.
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
        <BaseButton @click="emit('copy', sent.text, null)">
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
      v-if="copyStatus && copyStatus.supplierId === null"
      role="status"
    >
      {{ copyStatus.text }}
    </p>

    <SupplierCard
      v-for="supplier in suppliers"
      :key="supplier.supplierId"
      :supplier="supplier"
      :unchecked="unchecked"
      :preview="preview?.supplierId === supplier.supplierId ? preview : null"
      :sending="sending"
      :error="forCard(supplier.supplierId)"
      :status="statusFor(supplier.supplierId)"
      @pick="(orderItemId, picked) => emit('pick', orderItemId, picked, supplier.supplierId)"
      @toggle-preview="emit('togglePreview', supplier.supplierId)"
      @send="emit('send', supplier.supplierId)"
      @copy="(text) => emit('copy', text, supplier.supplierId)"
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
