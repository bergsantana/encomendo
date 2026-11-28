<script setup lang="ts">
/**
 * OrderView is "Pedido P-0231": the header, the delay banner, "Entrega", the
 * item × deadline timeline, "Itens" with their manual moves, "Próximo
 * status", "Pagamentos" and "Histórico". Every role that reads orders opens
 * it; each button needs its own right (`order:write`, `order:manage` for
 * "Alterar data", cancelling, the backup supplier and changing a paid or
 * discounted order, `order:override` for voiding and releasing). The buttons
 * only hide what a role cannot do; the API refuses it again. Cost and margin
 * show only when the API sent them.
 */
import type { Order } from '@encomendo/api'
import { BaseButton } from '@encomendo/ui'
import { storeToRefs } from 'pinia'
import { computed, onMounted, ref, watch } from 'vue'
import { RouterLink, useRoute } from 'vue-router'

import { usePermission } from '../../../composables/usePermission'
import { formatMoney } from '../../../format'
import AdvanceButtons from '../components/AdvanceButtons.vue'
import DateDialog from '../components/DateDialog.vue'
import DeadlineTimeline from '../components/DeadlineTimeline.vue'
import DelayBanner from '../components/DelayBanner.vue'
import DeliveryInfo from '../components/DeliveryInfo.vue'
import HandoverDialog from '../components/HandoverDialog.vue'
import ItemsTable from '../components/ItemsTable.vue'
import OrderHeader from '../components/OrderHeader.vue'
import OrderHistory from '../components/OrderHistory.vue'
import PaymentDialog from '../components/PaymentDialog.vue'
import PaymentsPanel from '../components/PaymentsPanel.vue'
import ReasonDialog from '../components/ReasonDialog.vue'
import ReleaseDialog from '../components/ReleaseDialog.vue'
import TrackingLinkButton from '../components/TrackingLinkButton.vue'
import type { Handover } from '../labels'
import { useOrderStore } from '../orderStore'
import { civilToday } from '../timeline'

const store = useOrderStore()
const { order, loading, loadError, timeZone, busy, notice, error, dialogError, guardMessage, conflict, history, hasMoreHistory, historyLoading, historyError } =
  storeToRefs(store)
const canWrite = usePermission('order:write')
const canManage = usePermission('order:manage')
const canOverride = usePermission('order:override')
const route = useRoute()

const orderId = (): string => String(route.params.orderId ?? '')

onMounted(() => store.open(orderId()))
// Leaving the screen changes the route's params before it unmounts: no order to read then.
watch(orderId, (id) => id !== '' && void store.open(id))

// A conflict leaves the dialog's stale form behind: close it, so "Recarregar" above is the way out.
watch(conflict, (stale) => stale && close())

type Dialog = 'payment' | 'void' | 'release' | 'date' | 'cancel' | 'handover'
const dialog = ref<Dialog | null>(null)
const target = ref<string | null>(null)

function show(which: Dialog, id: string | null = null): void {
  store.clearDialogError()
  target.value = id
  dialog.value = which
}

function close(): void {
  store.clearDialogError()
  dialog.value = null
  target.value = null
}

/** isOpen is true while the order can still change: neither handed over nor cancelled. */
function isOpen(current: Order): boolean {
  return current.status.phase !== 'handed_over' && current.status.phase !== 'cancelled'
}

const open = computed(() => order.value !== null && isOpen(order.value))
const today = computed(() => civilToday(timeZone.value))

/**
 * canChange says whether "Alterar pedido" is offered: before assembling, and
 * only to `order:manage` once the order is paid or discounted (the API
 * decides in the end).
 */
const canChange = computed(() => {
  const current = order.value
  if (current === null || !isOpen(current) || current.editable === 'closed') return false
  if (canManage.value) return true
  return canWrite.value && current.editable === 'free' && current.discount.cents === 0
})

const canRelease = computed(() => {
  const current = order.value
  if (current === null || !canOverride.value || current.depositPaid || current.released) return false
  return current.status.phase === 'draft' || current.status.phase === 'awaiting_deposit'
})

const voidedPayment = computed(() => order.value?.payments.find((payment) => payment.id === target.value))
const movingTo = computed(() => order.value?.moves.find((move) => move.key === target.value))

/** advance asks how the order was handed over before the move that hands it over; every other move goes at once. */
async function advance(move: Order['moves'][number]): Promise<void> {
  if (move.phase === 'handed_over') show('handover', move.key)
  else await store.advance(move.key)
}

async function handOver(handover: Handover): Promise<void> {
  const key = target.value
  if (key !== null && (await store.advance(key, handover))) close()
}

async function done(finished: Promise<boolean>): Promise<void> {
  if (await finished) close()
}

const money = (value: { cents: number; currency: string }) => formatMoney(value.cents, value.currency)
</script>

<template>
  <section class="order">
    <p
      v-if="loading"
      class="muted"
    >
      Carregando o pedido…
    </p>
    <template v-else-if="loadError">
      <p role="alert">
        {{ loadError }}
      </p>
      <div class="buttons">
        <BaseButton @click="store.open(orderId())">
          Tentar de novo
        </BaseButton>
        <RouterLink
          :to="{ name: 'orders' }"
          class="link-button"
        >
          Voltar para Pedidos
        </RouterLink>
      </div>
    </template>

    <template v-else-if="order">
      <OrderHeader
        :order="order"
        :time-zone="timeZone"
      >
        <RouterLink
          v-if="canChange"
          :to="{ name: 'change-order', params: { orderId: order.id } }"
          class="link-button"
        >
          Alterar pedido
        </RouterLink>
        <BaseButton
          v-if="open && canManage"
          @click="show('cancel')"
        >
          Cancelar pedido
        </BaseButton>
        <TrackingLinkButton
          v-if="open && canWrite"
          :busy="busy"
          @send="store.sendTracking()"
        />
      </OrderHeader>

      <p
        v-if="notice"
        role="status"
        class="notice"
      >
        {{ notice }}
      </p>
      <div
        v-if="error"
        class="alert"
        role="alert"
      >
        <p>{{ error }}</p>
        <BaseButton
          v-if="conflict"
          @click="store.reload()"
        >
          Recarregar
        </BaseButton>
      </div>

      <DelayBanner
        v-if="open"
        :order="order"
        :can-fix="canManage"
        :busy="busy"
        @use-backup="store.switchSupplier($event)"
        @change-date="show('date')"
      />
      <DeliveryInfo
        :order="order"
        :can-change-date="open && canManage"
        @change-date="show('date')"
      />
      <DeadlineTimeline
        :order="order"
        :today="today"
      />
      <ItemsTable
        :items="order.items"
        :can-move="open && canWrite"
        :busy="busy"
        @move="(itemId, to, check) => store.moveItem(itemId, to, check)"
      />
      <AdvanceButtons
        v-if="open && canWrite"
        :moves="order.moves"
        :guard-message="guardMessage"
        :busy="busy"
        @advance="advance"
      />
      <PaymentsPanel
        :order="order"
        :time-zone="timeZone"
        :can-record="open && canWrite"
        :can-void="canOverride"
        :can-release="canRelease"
        :busy="busy"
        @record="show('payment')"
        @void="show('void', $event)"
        @release="show('release')"
      />
      <OrderHistory
        :entries="history"
        :items="order.items"
        :time-zone="timeZone"
        :loading="historyLoading"
        :error="historyError"
        :has-more="hasMoreHistory"
        @more="store.loadMoreHistory()"
      />

      <PaymentDialog
        v-if="dialog === 'payment'"
        :order="order"
        :error="dialogError"
        :busy="busy"
        @submit="(payment) => done(store.pay(payment))"
        @close="close"
      />
      <ReasonDialog
        v-if="dialog === 'void' && voidedPayment"
        :title="`Anular o pagamento de ${money(voidedPayment.amount)}?`"
        confirm="Anular pagamento"
        :error="dialogError"
        :busy="busy"
        @submit="(reason) => done(store.annul(voidedPayment?.id ?? '', reason))"
        @close="close"
      />
      <ReleaseDialog
        v-if="dialog === 'release'"
        :error="dialogError"
        :busy="busy"
        @confirm="done(store.release())"
        @close="close"
      />
      <DateDialog
        v-if="dialog === 'date'"
        :current="order.deliveryDate"
        :error="dialogError"
        :busy="busy"
        @submit="(date, reason) => done(store.changeDate(date, reason))"
        @close="close"
      />
      <ReasonDialog
        v-if="dialog === 'cancel'"
        :title="`Cancelar o pedido ${order.number}?`"
        confirm="Cancelar pedido"
        :error="dialogError"
        :busy="busy"
        @submit="(reason) => done(store.cancel(reason))"
        @close="close"
      >
        <p
          v-if="order.paid.cents > 0"
          class="muted"
        >
          Os pagamentos registrados continuam no pedido. A devolução é feita fora do Encomendo.
        </p>
      </ReasonDialog>
      <HandoverDialog
        v-if="dialog === 'handover' && movingTo"
        :status="movingTo.label"
        :planned="order.handover"
        :error="guardMessage ?? error"
        :busy="busy"
        @confirm="handOver"
        @close="close"
      />
    </template>
  </section>
</template>

<style scoped>
.order {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 20px;
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
.notice {
  font-weight: 600;
}
.alert {
  display: flex;
  align-items: center;
  gap: 12px;
}
.buttons {
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

/* Phone: one column, as wide as the screen; the actions one under the other. */
@media (max-width: 640px) {
  .order {
    align-items: stretch;
  }
  .alert {
    flex-wrap: wrap;
  }
  .link-button {
    justify-content: center;
  }
}
</style>
