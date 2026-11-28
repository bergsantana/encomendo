<script setup lang="ts">
/**
 * OrderFormView is "Novo pedido" and "Alterar pedido P-0231" (the same form
 * on two addresses). It composes the customer, the lines, "Entrega" and
 * "Valores", and "Resumo", which is only what the quote answered. Saving a
 * new order ends in "Pedido P-0231 criado." with the tracking link and
 * "Enviar pelo WhatsApp"; changing a paid order asks for "O cliente aprovou a
 * mudança" first. Leaving with edits asks "Descartar as alterações?".
 */
import { BaseButton } from '@encomendo/ui'
import { storeToRefs } from 'pinia'
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { onBeforeRouteLeave, RouterLink, useRoute, useRouter } from 'vue-router'

import { niche } from '../../../niche'
import DiscardQuestion from '../../catalogo/components/DiscardQuestion.vue'
import CatalogPicker from '../components/CatalogPicker.vue'
import ChangeSummary from '../components/ChangeSummary.vue'
import CustomerPicker from '../components/CustomerPicker.vue'
import LineEditor from '../components/LineEditor.vue'
import QuoteSummary from '../components/QuoteSummary.vue'
import TermsFields from '../components/TermsFields.vue'
import { useOrderFormStore } from '../formStore'

const store = useOrderFormStore()
const {
  order,
  loading,
  loadError,
  settings,
  customer,
  newCustomer,
  draft,
  kits,
  items,
  quote,
  quoting,
  quoteError,
  archivedNames,
  needsApproval,
  canManage,
  saving,
  formError,
  fieldErrors,
  conflict,
  created,
  changed,
  notice,
  dirty,
  customerResults,
  customerSearching,
  customerError,
  customerFieldErrors,
  catalogResults,
  catalogSearching,
  catalogError,
} = storeToRefs(store)
const route = useRoute()
const router = useRouter()

const changing = computed(() => route.name === 'change-order')
const orderId = (): string => String(route.params.orderId ?? '')

async function start(): Promise<void> {
  if (changing.value) await store.startChange(orderId())
  else await store.startNew()
}

onMounted(start)
onBeforeUnmount(() => store.stop())
watch(() => route.fullPath, () => void start())

/** closedMessage says why an order past the point of change cannot be changed. */
const closedMessage = computed(() => {
  const phase = order.value?.status.phase
  if (phase === 'handed_over' || phase === 'cancelled') return 'Este pedido já foi entregue ou cancelado.'
  return `Este pedido já está em ${niche.montagem} e não pode mais ser alterado.`
})

/**
 * lockedForStaff is true for a paid or discounted order opened by a role
 * without `order:manage`: only the owner and admins change it (ADR 0030).
 */
const lockedForStaff = computed(() => {
  const current = order.value
  if (current === null || canManage.value) return false
  return current.editable === 'approval' || current.discount.cents > 0
})

const title = computed(() => (changing.value && order.value !== null ? `Alterar pedido ${order.value.number}` : 'Novo pedido'))

// ---- The catalog dialogs ----

const picking = ref<'kits' | 'items' | null>(null)

async function choose(id: string): Promise<void> {
  const added = picking.value === 'kits' ? await store.addKit(id) : await store.addItem(id)
  if (added) picking.value = null
}

// ---- Leaving with edits ----

const asking = ref(false)
let answer: ((leave: boolean) => void) | null = null

onBeforeRouteLeave(() => {
  if (!dirty.value) return true
  asking.value = true
  return new Promise<boolean>((resolve) => {
    answer = resolve
  })
})

function settle(leave: boolean): void {
  asking.value = false
  answer?.(leave)
  answer = null
}

function cancel(): void {
  void router.push({ name: order.value === null ? 'orders' : 'order', params: order.value === null ? {} : { orderId: order.value.id } })
}

function quotedLine(index: number) {
  return quote.value?.lines[index]
}

</script>

<template>
  <section class="order-form">
    <h1>{{ title }}</h1>

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
      <BaseButton @click="start">
        Tentar de novo
      </BaseButton>
    </template>

    <!-- After creating -->
    <div
      v-else-if="created"
      class="done"
    >
      <p role="status">
        Pedido {{ created.order.number }} criado.
      </p>
      <p
        v-if="notice"
        role="alert"
      >
        {{ notice }}
      </p>
      <p v-if="created.trackingUrl">
        Link de acompanhamento: <span class="link-text">{{ created.trackingUrl }}</span>
      </p>
      <div class="buttons">
        <BaseButton
          variant="primary"
          @click="store.sendTracking()"
        >
          Enviar pelo WhatsApp
        </BaseButton>
        <RouterLink
          :to="{ name: 'order', params: { orderId: created.order.id } }"
          class="link-button"
        >
          Abrir pedido
        </RouterLink>
        <RouterLink
          :to="{ name: 'orders' }"
          class="link-button"
        >
          Voltar para Pedidos
        </RouterLink>
      </div>
    </div>

    <!-- After changing -->
    <div
      v-else-if="changed && order"
      class="done"
    >
      <p role="status">
        Pedido alterado.
      </p>
      <ChangeSummary
        v-if="changed.change.paid"
        :order="order"
        :quote="null"
        :changed="changed"
      />
      <div class="buttons">
        <BaseButton
          v-if="changed.whatsappUrl"
          variant="primary"
          @click="store.openChangeMessage()"
        >
          Avisar o cliente pelo WhatsApp
        </BaseButton>
        <RouterLink
          :to="{ name: 'order', params: { orderId: changed.order.id } }"
          class="link-button"
        >
          Abrir pedido
        </RouterLink>
      </div>
    </div>

    <!-- A change that cannot be made -->
    <div
      v-else-if="changing && order && order.editable === 'closed'"
      class="done"
    >
      <p role="alert">
        {{ closedMessage }}
      </p>
      <RouterLink
        :to="{ name: 'order', params: { orderId: order.id } }"
        class="link-button"
      >
        Abrir pedido
      </RouterLink>
    </div>
    <div
      v-else-if="changing && lockedForStaff && order"
      class="done"
    >
      <p role="alert">
        Só o Dono ou um Admin altera um pedido com pagamento ou desconto.
      </p>
      <RouterLink
        :to="{ name: 'order', params: { orderId: order.id } }"
        class="link-button"
      >
        Abrir pedido
      </RouterLink>
    </div>

    <form
      v-else
      class="form"
      novalidate
      @submit.prevent="store.save(false)"
    >
      <DiscardQuestion
        v-if="asking"
        @keep="settle(false)"
        @discard="settle(true)"
      />

      <p
        v-if="needsApproval"
        class="notice"
      >
        Este pedido já tem pagamento. Combine a mudança com o cliente antes de salvar.
      </p>

      <div
        v-if="formError"
        class="alert"
        role="alert"
      >
        <p>{{ formError }}</p>
        <BaseButton
          v-if="conflict"
          @click="store.reload()"
        >
          Recarregar
        </BaseButton>
      </div>

      <CustomerPicker
        :chosen="customer ?? newCustomer"
        :locked="changing"
        :results="customerResults"
        :searching="customerSearching"
        :error="customerError"
        :field-errors="customerFieldErrors"
        :order-error="fieldErrors.customer ?? null"
        @search="store.searchCustomers($event)"
        @choose="store.chooseCustomer($event)"
        @clear="store.clearCustomer()"
        @create="(name, phone) => store.useNewCustomer(name, phone)"
      />

      <section
        class="lines"
        aria-labelledby="items-heading"
      >
        <h2 id="items-heading">
          Itens
        </h2>
        <p
          v-if="fieldErrors.lines"
          class="error"
        >
          {{ fieldErrors.lines }}
        </p>
        <ul
          v-if="draft.lines.length > 0"
          class="line-list"
        >
          <LineEditor
            v-for="(line, index) in draft.lines"
            :key="line.key"
            :line="line"
            :kit="kits[line.catalogId]"
            :items="items"
            :quoted="quotedLine(index)"
            :errors="fieldErrors"
            :archived-names="archivedNames"
            @quantity="line.quantity = $event"
            @swap="(itemId, withItemId) => store.swap(line.key, itemId, withItemId)"
            @choose="(itemId, option, choiceId) => store.choose(line.key, itemId, option, choiceId)"
            @remove="store.removeLine(line.key)"
          />
        </ul>
        <div class="buttons">
          <BaseButton @click="picking = 'kits'">
            Adicionar kit
          </BaseButton>
          <BaseButton @click="picking = 'items'">
            Adicionar item
          </BaseButton>
        </div>
      </section>

      <TermsFields
        v-model="draft.terms"
        :addresses="customer?.addresses ?? []"
        :can-manage="canManage"
        :defaults="settings"
        :errors="fieldErrors"
      />

      <QuoteSummary
        :quote="quote"
        :quoting="quoting"
        :error="quoteError"
      />
      <ChangeSummary
        v-if="order && needsApproval"
        :order="order"
        :quote="quote"
        :changed="null"
      />

      <label
        v-if="needsApproval"
        class="enc-check approval"
      >
        <input
          v-model="draft.customerApproved"
          type="checkbox"
          :aria-invalid="fieldErrors.approval ? 'true' : undefined"
          :aria-describedby="fieldErrors.approval ? 'approval-error' : undefined"
        >
        O cliente aprovou a mudança
      </label>
      <p
        v-if="fieldErrors.approval"
        id="approval-error"
        role="alert"
        class="error"
      >
        {{ fieldErrors.approval }}
      </p>

      <div class="buttons actions">
        <template v-if="changing">
          <BaseButton
            variant="primary"
            type="submit"
            :disabled="saving"
          >
            Salvar alterações
          </BaseButton>
        </template>
        <template v-else>
          <BaseButton
            type="submit"
            :disabled="saving"
          >
            Salvar rascunho
          </BaseButton>
          <BaseButton
            variant="primary"
            :disabled="saving"
            @click="store.save(true)"
          >
            Salvar e enviar ao cliente
          </BaseButton>
        </template>
        <BaseButton @click="cancel">
          Cancelar
        </BaseButton>
      </div>
    </form>

    <CatalogPicker
      v-if="picking"
      :what="picking"
      :kits="catalogResults.kits"
      :items="catalogResults.items"
      :searching="catalogSearching"
      :error="catalogError"
      @search="store.searchCatalog(picking, $event)"
      @choose="choose"
      @close="picking = null"
    />
  </section>
</template>

<style scoped>
.order-form {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 16px;
}
.form,
.done {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 20px;
  width: 100%;
}
.lines {
  display: flex;
  flex-direction: column;
  gap: 12px;
  width: 100%;
  max-width: 720px;
}
.line-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
  margin: 0;
  padding: 0;
  list-style: none;
}
h1,
h2,
p {
  margin: 0;
}
h2 {
  font-size: 15px;
}
.muted {
  color: var(--enc-ink-secondary);
}
.error,
[role='alert'] {
  color: var(--enc-status-alert-text);
  font-weight: 600;
}
.notice {
  padding: 8px 12px;
  border-radius: var(--enc-radius-card);
  background: var(--enc-status-waiting-fill);
  color: var(--enc-status-waiting-text);
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
  align-items: center;
  gap: 8px;
}
.link-text {
  font-family: var(--enc-font-mono);
  overflow-wrap: anywhere;
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

/* Phone: one column, the buttons one under the other and as wide as the screen. */
@media (max-width: 640px) {
  .order-form,
  .form,
  .done {
    align-items: stretch;
  }
  .alert {
    flex-wrap: wrap;
  }
  .actions {
    flex-direction: column;
    align-items: stretch;
  }
  .link-button {
    justify-content: center;
  }
}
</style>
