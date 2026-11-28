<script setup lang="ts">
/**
 * PedidosView is the order list: tabs by status, search, "Só com alerta"
 * and "Novo pedido". Every role that reads orders (`order:read`) opens it;
 * "Novo pedido" needs `order:write`. A link can preset the list (Hoje's
 * cards do): `?status={key}`, `?scope=handed_over` or `cancelled`, and
 * `?phase={phase}` and `?alert=true`; the address follows the tab and the alert filter, so a reload
 * lands on the same list.
 */
import { BaseButton, isPhase } from '@encomendo/ui'
import { storeToRefs } from 'pinia'
import { computed, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import { usePermission } from '../../../composables/usePermission'
import OrdersToolbar from '../components/OrdersToolbar.vue'
import OrderTable from '../components/OrderTable.vue'
import StatusTabs from '../components/StatusTabs.vue'
import { activeTab, cancelledTab, handedOverTab, statusTabId, tabElementId, usePedidosStore } from '../store'

const store = usePedidosStore()
const { items, tabs, hasMore, loading, loaded, loadFailed, error, tab, query, alertOnly, phase, phaseLabel } = storeToRefs(store)
const canWrite = usePermission('order:write')
const route = useRoute()
const router = useRouter()

/** tabFromQuery turns the address into a tab: a status key, or the scope of "Entregues" and "Cancelados". */
function tabFromQuery(): string {
  const { status, scope } = route.query
  if (typeof status === 'string' && status !== '') return statusTabId(status)
  if (scope === 'handed_over') return handedOverTab
  if (scope === 'cancelled') return cancelledTab
  return activeTab
}

/** phaseFromQuery reads `?phase=` when it names a known phase. */
function phaseFromQuery() {
  const { phase: asked } = route.query
  return typeof asked === 'string' && isPhase(asked) ? asked : undefined
}

/** queryForState is the address of the list as it stands. */
function queryForState(): Record<string, string> {
  const query: Record<string, string> = {}
  if (tab.value.startsWith('status:')) query.status = tab.value.slice('status:'.length)
  else if (tab.value === handedOverTab || tab.value === cancelledTab) query.scope = tab.value
  if (phase.value !== undefined) query.phase = phase.value
  if (alertOnly.value) query.alert = 'true'
  return query
}

async function openFromAddress(): Promise<void> {
  await store.open({ tab: tabFromQuery(), phase: phaseFromQuery(), alertOnly: route.query.alert === 'true' || route.query.alert === '1' })
}

onMounted(openFromAddress)

// A link into Pedidos while the list is open (Hoje's cards) presets it again.
watch(
  () => route.fullPath,
  () => {
    const wanted = JSON.stringify(Object.entries(queryForState()).sort())
    const asked = JSON.stringify(Object.entries(route.query).sort())
    if (route.name === 'orders' && wanted !== asked) void openFromAddress()
  },
)

async function refresh(): Promise<void> {
  await store.reload()
  await router.replace({ name: 'orders', query: queryForState() })
}

const selectedTab = computed({
  get: () => tab.value,
  set: (id: string) => {
    tab.value = id
    phase.value = undefined
    void refresh()
  },
})

const noFilter = (): boolean => query.value.trim() === '' && !alertOnly.value && phase.value === undefined && tab.value === activeTab
const empty = computed(() => loaded.value && items.value.length === 0 && !error.value && noFilter())

async function clearPhase(): Promise<void> {
  await store.clearPhase()
  await router.replace({ name: 'orders', query: queryForState() })
}

function newOrder(): void {
  void router.push({ name: 'new-order' })
}
</script>

<template>
  <section class="pedidos">
    <h1>Pedidos</h1>

    <OrdersToolbar
      v-model:query="query"
      v-model:alert-only="alertOnly"
      :can-write="canWrite && !empty"
      @search="refresh"
      @new-order="newOrder"
    />
    <StatusTabs
      v-model="selectedTab"
      :tabs="tabs"
    />

    <div
      v-if="phase !== undefined && phaseLabel"
      class="chips"
    >
      <button
        type="button"
        class="chip"
        :aria-label="`Tirar o filtro ${phaseLabel}`"
        @click="clearPhase"
      >
        {{ phaseLabel }} ×
      </button>
    </div>

    <div
      id="orders-panel"
      class="panel"
      role="tabpanel"
      :aria-labelledby="tabElementId(tab)"
    >
      <p
        v-if="error"
        role="alert"
      >
        {{ error }}
      </p>
      <BaseButton
        v-if="loadFailed"
        @click="store.reload()"
      >
        Tentar de novo
      </BaseButton>

      <OrderTable
        v-if="items.length > 0"
        :items="items"
      />
      <p
        v-else-if="(loading || !loaded) && !error"
        class="muted"
      >
        Carregando os pedidos…
      </p>
      <template v-else-if="!error">
        <p class="muted">
          {{ empty ? 'Nenhum pedido ainda.' : 'Nenhum pedido encontrado.' }}
        </p>
        <BaseButton
          v-if="empty && canWrite"
          variant="primary"
          @click="newOrder"
        >
          Novo pedido
        </BaseButton>
      </template>
      <BaseButton
        v-if="hasMore"
        :disabled="loading"
        @click="store.loadMore()"
      >
        Carregar mais
      </BaseButton>
    </div>
  </section>
</template>

<style scoped>
.pedidos {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 16px;
}
.panel {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 16px;
  width: 100%;
}
h1,
p {
  margin: 0;
}
[role='alert'] {
  font-weight: 600;
}
.chip {
  min-height: var(--enc-control-height);
  padding: 0 12px;
  border: 1px solid var(--enc-secondary-border);
  border-radius: var(--enc-radius-badge);
  background: var(--enc-surface);
  color: var(--enc-ink);
  font: inherit;
  font-weight: 600;
  cursor: pointer;
}
.muted {
  color: var(--enc-ink-secondary);
}

@media (max-width: 640px) {
  .pedidos,
  .panel {
    align-items: stretch;
  }
}
</style>
