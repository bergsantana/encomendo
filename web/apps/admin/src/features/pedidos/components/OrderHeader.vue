<script setup lang="ts">
/**
 * OrderHeader is "Pedido P-0231": the status badge (the words of the order's
 * own workflow, the color of its phase), the customer with the WhatsApp
 * number, and "Criado em {data}". Its slot holds the order's actions.
 */
import type { Order } from '@encomendo/api'
import { BaseButton, StatusBadge } from '@encomendo/ui'

import { formatDate } from '../../../format'
import { formatWhatsapp } from '../../../phone'

defineProps<{
  order: Order
  /** timeZone is the store's, so "Criado em" shows the day the store lived it. */
  timeZone?: string
}>()

const emit = defineEmits<{
  /** chat asks to open the customer's chat, with the link the API sent. */
  chat: []
}>()
</script>

<template>
  <header class="header">
    <div class="title">
      <h1>Pedido <span class="enc-code">{{ order.number }}</span></h1>
      <StatusBadge
        :label="order.status.label"
        :phase="order.status.phase"
      />
    </div>
    <p class="customer">
      <strong>{{ order.customer.name }}</strong>
      <span class="muted">{{ formatWhatsapp(order.customer.phone) }}</span>
      <BaseButton @click="emit('chat')">
        Abrir conversa no WhatsApp
      </BaseButton>
    </p>
    <p class="muted">
      Criado em {{ formatDate(order.createdAt, timeZone) }}
    </p>
    <div class="actions">
      <slot />
    </div>
  </header>
</template>

<style scoped>
.header {
  display: flex;
  flex-direction: column;
  gap: 8px;
  width: 100%;
}
.title {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 12px;
}
h1,
p {
  margin: 0;
}
.customer {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 12px;
}
.muted {
  color: var(--enc-ink-secondary);
}
.actions {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  gap: 8px 16px;
}

@media (max-width: 640px) {
  .actions {
    flex-direction: column;
    align-items: stretch;
  }
}
</style>
