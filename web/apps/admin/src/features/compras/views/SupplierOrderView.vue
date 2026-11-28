<script setup lang="ts">
/**
 * SupplierOrderView is "Pedido de compra C-0012": the supplier with "Abrir
 * conversa no WhatsApp", who sent it and when, the supplier's promised date,
 * and "Itens". The actions follow the pedido: "Informar prazo", "Marcar como
 * em trânsito", "Receber itens" (opens Receber itens narrowed to this pedido,
 * for `receiving:write`, while it has open lines), "Informar preço pago" (the
 * owner's) and "Desfazer envio" while nothing was received. Compras needs `purchase:write`; the API checks every
 * write again. Cost and paid price show only when the API sent them.
 */
import { BaseButton, StatusBadge } from '@encomendo/ui'
import { storeToRefs } from 'pinia'
import { computed, onMounted, ref, watch } from 'vue'
import { RouterLink, useRoute } from 'vue-router'

import { usePermission } from '../../../composables/usePermission'
import { formatCivilDate, formatDate } from '../../../format'
import PaidPricesDialog from '../components/PaidPricesDialog.vue'
import PromisedDateDialog from '../components/PromisedDateDialog.vue'
import SupplierOrderLines from '../components/SupplierOrderLines.vue'
import UndoDialog from '../components/UndoDialog.vue'
import { orderStateLabel, orderStateTone, receivedMessage } from '../labels'
import { useComprasStore } from '../store'

const store = useComprasStore()
const { detail, detailLoading, detailError, busy, notice, error, dialogError, conflict, delayWarnings, timeZone, sendsCosts } = storeToRefs(store)
const route = useRoute()
const canReceive = usePermission('receiving:write')
const dialog = ref<'date' | 'prices' | 'undo' | null>(null)

const supplierOrderId = (): string => String(route.params.supplierOrderId ?? '')

onMounted(() => store.openOrder(supplierOrderId()))
// Leaving the screen changes the route's params before it unmounts: no pedido to read then.
watch(supplierOrderId, (id) => id !== '' && void store.openOrder(id))

const undone = computed(() => detail.value?.state === 'undone')
const openLines = computed(() => detail.value?.lines.filter((line) => line.status === 'sent' || line.status === 'in_transit') ?? [])
const pricedLines = computed(() => detail.value?.lines.filter((line) => line.status !== 'cancelled') ?? [])
const canMarkInTransit = computed(() => openLines.value.some((line) => line.status === 'sent'))

function show(next: 'date' | 'prices' | 'undo'): void {
  store.clearDialogError()
  dialog.value = next
}

function close(): void {
  dialog.value = null
  store.clearDialogError()
}

/**
 * done closes the dialog once its write went through, and also on a version
 * conflict: that message and "Recarregar" are on the page, behind a modal
 * dialog they could not be reached. Any other refusal stays in the dialog.
 */
async function done(write: Promise<boolean>): Promise<void> {
  if ((await write) || conflict.value) dialog.value = null
}
</script>

<template>
  <section class="supplier-order">
    <p
      v-if="detailLoading"
      class="muted"
    >
      Carregando as compras…
    </p>

    <template v-else-if="detailError">
      <p role="alert">
        {{ detailError }}
      </p>
      <RouterLink
        :to="{ name: 'purchasing', query: { tab: 'enviadas' } }"
        class="link-button"
      >
        Voltar para Compras
      </RouterLink>
    </template>

    <template v-else-if="detail">
      <header class="header">
        <div class="title">
          <h1>Pedido de compra <span class="enc-code">{{ detail.number }}</span></h1>
          <StatusBadge
            :label="orderStateLabel(detail.state)"
            :phase="orderStateTone(detail.state)"
          />
        </div>
        <p class="supplier">
          <strong>{{ detail.supplierName }}</strong>
          <BaseButton @click="store.openChat()">
            Abrir conversa no WhatsApp
          </BaseButton>
        </p>
        <p class="muted">
          Enviado em {{ formatDate(detail.sentAt, timeZone) }} por {{ detail.sentBy.name }}
        </p>
        <p class="muted">
          Prazo do fornecedor: {{ detail.promisedDate ? formatCivilDate(detail.promisedDate) : 'Sem prazo' }}
        </p>
        <div
          v-if="!undone"
          class="actions"
        >
          <BaseButton
            v-if="openLines.length > 0"
            :disabled="busy"
            @click="show('date')"
          >
            Informar prazo
          </BaseButton>
          <BaseButton
            v-if="canMarkInTransit"
            :disabled="busy"
            @click="store.markOrderInTransit()"
          >
            Marcar como em trânsito
          </BaseButton>
          <RouterLink
            v-if="canReceive && openLines.length > 0"
            class="link-button"
            :to="{ name: 'receive', query: { pedido: detail.id } }"
          >
            Receber itens
          </RouterLink>
          <BaseButton
            v-if="sendsCosts && pricedLines.length > 0"
            :disabled="busy"
            @click="show('prices')"
          >
            Informar preço pago
          </BaseButton>
          <BaseButton
            v-if="detail.canUndo"
            :disabled="busy"
            @click="show('undo')"
          >
            Desfazer envio
          </BaseButton>
        </div>
        <p
          v-if="!undone && !detail.canUndo"
          class="muted"
        >
          {{ receivedMessage }}
        </p>
      </header>

      <p
        v-if="notice"
        role="status"
        class="strong"
      >
        {{ notice }}
      </p>
      <ul
        v-if="delayWarnings.length > 0"
        class="warnings"
      >
        <li
          v-for="warning in delayWarnings"
          :key="warning.lineId"
          role="alert"
        >
          {{ warning.itemName }} do pedido
          <RouterLink
            class="enc-code"
            :to="{ name: 'order', params: { orderId: warning.orderId } }"
          >
            {{ warning.orderNumber }}
          </RouterLink>
          pode atrasar a entrega. Veja o pedido para resolver.
        </li>
      </ul>
      <div
        v-if="error"
        class="alert"
        role="alert"
      >
        <p>{{ error }}</p>
        <BaseButton
          v-if="conflict"
          @click="store.reloadOrder()"
        >
          Recarregar
        </BaseButton>
      </div>

      <section
        class="items"
        aria-labelledby="supplier-order-items"
      >
        <h2 id="supplier-order-items">
          Itens
        </h2>
        <SupplierOrderLines :lines="detail.lines" />
      </section>

      <PromisedDateDialog
        v-if="dialog === 'date'"
        :lines="openLines"
        :current="detail.promisedDate"
        :error="dialogError"
        :busy="busy"
        @submit="(date, lines) => done(store.savePromisedDate(date, lines))"
        @close="close"
      />
      <PaidPricesDialog
        v-if="dialog === 'prices'"
        :lines="pricedLines"
        :error="dialogError"
        :busy="busy"
        @submit="(prices) => done(store.savePaidPrices(prices))"
        @close="close"
      />
      <UndoDialog
        v-if="dialog === 'undo'"
        :number="detail.number"
        :error="dialogError"
        :busy="busy"
        @confirm="done(store.undo())"
        @close="close"
      />
    </template>
  </section>
</template>

<style scoped>
.supplier-order {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 16px;
}
h1,
h2,
p {
  margin: 0;
}
h2 {
  font-size: 16px;
}
.muted {
  color: var(--enc-ink-secondary);
}
.strong {
  font-weight: 600;
}
[role='alert'] {
  color: var(--enc-status-alert-text);
  font-weight: 600;
}
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
.supplier {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px 12px;
}
.actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 16px;
}
.alert {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 12px;
}
.warnings {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin: 0;
  padding: 0;
  list-style: none;
}
.items {
  display: flex;
  flex-direction: column;
  gap: 8px;
  width: 100%;
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

/* Phone: the actions one under the other, as wide as the screen. */
@media (max-width: 640px) {
  .actions > .enc-button {
    flex: 1 1 100%;
  }
  .link-button {
    justify-content: center;
  }
}
</style>
