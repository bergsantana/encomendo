<script setup lang="ts">
/**
 * ComprasView is Compras: the tabs "A comprar", "Enviadas", "Receber itens"
 * and "Fornecedores", with "Avisar o fornecedor" above them when a sent line
 * is no longer needed. `?tab=` keeps the tab, so a reload or a pasted link
 * lands on the same one. Owner and admin open it (`purchase:write`); suppliers
 * can be created and changed with `catalog:write`, which both have.
 */
import type { Supplier } from '@encomendo/api'
import { storeToRefs } from 'pinia'
import { computed, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import { usePermission } from '../../../composables/usePermission'
import CatalogTabs from '../../catalogo/components/CatalogTabs.vue'
import AComprarTab from '../components/AComprarTab.vue'
import EnviadasTab from '../components/EnviadasTab.vue'
import FornecedoresTab from '../components/FornecedoresTab.vue'
import NoticesPanel from '../components/NoticesPanel.vue'
import SupplierDrawer from '../components/SupplierDrawer.vue'
import { type ComprasTab, useComprasStore } from '../store'

const store = useComprasStore()
const {
  suppliers,
  unchecked,
  toBuyLoaded,
  toBuyLoading,
  toBuyError,
  purchaseBeforeDeposit,
  preview,
  sending,
  sent,
  copyStatus,
  cardError,
  truncated,
  orders,
  ordersFilter,
  ordersLoaded,
  ordersLoading,
  ordersError,
  hasMoreOrders,
  timeZone,
  notices,
  noticeBusy,
  noticeStatus,
  noticeError,
  directory,
  directoryLoaded,
  directoryLoading,
  directoryError,
  hasMoreSuppliers,
  query,
  supplierNotice,
  editing,
  drawerOpen,
  drawerKey,
  supplierConflict,
  supplierItemList,
  supplierItemsLoading,
  supplierItemsError,
  savingSupplier,
  supplierFormError,
  supplierFieldErrors,
} = storeToRefs(store)
const canWriteSuppliers = usePermission('catalog:write')
const route = useRoute()
const router = useRouter()

const tabs: { id: ComprasTab; label: string }[] = [
  { id: 'a-comprar', label: 'A comprar' },
  { id: 'enviadas', label: 'Enviadas' },
  { id: 'receber', label: 'Receber itens' },
  { id: 'fornecedores', label: 'Fornecedores' },
]

/** tab is the one `?tab=` names; anything else is the first. */
const tab = computed<ComprasTab>({
  get: () => tabs.find((candidate) => candidate.id === route.query.tab)?.id ?? 'a-comprar',
  set: (id) => void router.replace({ name: 'purchasing', query: id === 'a-comprar' ? {} : { tab: id } }),
})

onMounted(() => store.loadNotices())
// Every time a tab is shown it reads its own data again: another device may have sent or received meanwhile.
watch(tab, (shown) => void store.openTab(shown), { immediate: true })

function saveSupplier(name: string, whatsapp: string): void {
  void store.saveSupplier(name, whatsapp)
}

function openSupplier(supplier: Supplier | null): void {
  store.openSupplier(supplier)
}
</script>

<template>
  <section class="compras">
    <h1>Compras</h1>

    <NoticesPanel
      :notices="notices"
      :busy-id="noticeBusy"
      :status="noticeStatus"
      :error="noticeError"
      @notify="store.openNotice($event)"
      @resolve="store.resolve($event)"
    />

    <CatalogTabs
      v-model="tab"
      :tabs="tabs"
      label="Compras"
    />

    <div
      v-if="tab === 'a-comprar'"
      id="panel-a-comprar"
      role="tabpanel"
      aria-labelledby="tab-a-comprar"
      class="panel"
    >
      <AComprarTab
        :suppliers="suppliers"
        :loaded="toBuyLoaded"
        :loading="toBuyLoading"
        :error="toBuyError"
        :unchecked="unchecked"
        :preview="preview"
        :sending="sending"
        :sent="sent"
        :copy-status="copyStatus"
        :card-error="cardError"
        :truncated="truncated"
        :purchase-before-deposit="purchaseBeforeDeposit"
        @pick="(orderItemId, picked, supplierId) => store.setPicked(supplierId, orderItemId, picked)"
        @toggle-preview="store.togglePreview($event)"
        @send="store.send($event)"
        @copy="(text, supplierId) => store.copyMessage(text, supplierId)"
        @open-link="store.openSentLink()"
        @retry="store.loadToBuy()"
      />
    </div>

    <div
      v-else-if="tab === 'enviadas'"
      id="panel-enviadas"
      role="tabpanel"
      aria-labelledby="tab-enviadas"
      class="panel"
    >
      <EnviadasTab
        :orders="orders"
        :filter="ordersFilter"
        :loaded="ordersLoaded"
        :loading="ordersLoading"
        :error="ordersError"
        :has-more="hasMoreOrders"
        :time-zone="timeZone"
        @filter="store.loadOrders($event)"
        @load-more="store.loadMoreOrders()"
      />
    </div>

    <!-- "Receber itens" is step 5.14's: its panel is empty until that screen lands. -->
    <div
      v-else-if="tab === 'receber'"
      id="panel-receber"
      role="tabpanel"
      aria-labelledby="tab-receber"
      class="panel"
    />

    <div
      v-else
      id="panel-fornecedores"
      role="tabpanel"
      aria-labelledby="tab-fornecedores"
      class="panel"
    >
      <FornecedoresTab
        v-model="query"
        :suppliers="directory"
        :loaded="directoryLoaded"
        :loading="directoryLoading"
        :error="directoryError"
        :has-more="hasMoreSuppliers"
        :notice="supplierNotice"
        :can-write="canWriteSuppliers"
        @search="store.loadSuppliers()"
        @open="openSupplier"
        @load-more="store.loadMoreSuppliers()"
      />
      <SupplierDrawer
        v-if="drawerOpen"
        :key="drawerKey"
        :supplier="editing"
        :items="supplierItemList"
        :items-loading="supplierItemsLoading"
        :items-error="supplierItemsError"
        :can-write="canWriteSuppliers"
        :saving="savingSupplier"
        :form-error="supplierFormError"
        :field-errors="supplierFieldErrors"
        :conflict="supplierConflict"
        @save="saveSupplier"
        @cancel="store.closeSupplier()"
        @reload="store.reloadSupplier()"
      />
    </div>
  </section>
</template>

<style scoped>
.compras {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 16px;
}
h1 {
  margin: 0;
}
.panel {
  width: 100%;
}
</style>
