<script setup lang="ts">
/**
 * ClientesView is the customer list. Every role that reads orders
 * (`order:read`) opens it; only owner and admin (`customer:write`) get the
 * form and "Novo cliente", the others read the customer as lines. A customer
 * has an address of its own (`/clientes/:customerId`) that opens the drawer
 * over the list, so a link or a reload lands on the same customer.
 */
import { BaseButton } from '@encomendo/ui'
import { storeToRefs } from 'pinia'
import { nextTick, onBeforeUnmount, onMounted, useTemplateRef, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import { usePermission } from '../../../composables/usePermission'
import CustomerForm from '../components/CustomerForm.vue'
import CustomersToolbar from '../components/CustomersToolbar.vue'
import CustomerTable from '../components/CustomerTable.vue'
import type { CustomerDraft } from '../customerForm'
import { useCustomersStore } from '../store'

const store = useCustomersStore()
const {
  items,
  hasMore,
  loading,
  loaded,
  loadFailed,
  query,
  notice,
  error,
  editing,
  drawerOpen,
  drawerKey,
  conflict,
  saving,
  formError,
  fieldErrors,
  orders,
  ordersLoading,
  ordersError,
  ordersCapped,
  timeZone,
} = storeToRefs(store)
const canWrite = usePermission('customer:write')
const route = useRoute()
const router = useRouter()
const status = useTemplateRef<HTMLElement>('status')

function routeCustomerId(): string | null {
  const id = route.params.customerId
  return typeof id === 'string' ? id : null
}

// The address decides what the drawer shows: a customer's own address opens
// that customer, the list's address closes the drawer.
watch(
  () => routeCustomerId(),
  (customerId) => {
    if (customerId !== null) void store.openCustomer(customerId)
    else if (editing.value !== null) store.closeDrawer()
  },
)

onMounted(async () => {
  const customerId = routeCustomerId()
  await Promise.all([store.open(), customerId === null ? undefined : store.openCustomer(customerId)])
  // A customer that is not there leaves the list with its message.
  if (customerId !== null && !drawerOpen.value) await router.replace({ name: 'customers' })
})

// Leaving the screen with the drawer open must not bring it back on return.
onBeforeUnmount(() => store.closeDrawer())

/** close shuts the drawer; a customer's own address goes back to the list. */
function close(): void {
  if (routeCustomerId() !== null) void router.push({ name: 'customers' })
  else store.closeDrawer()
}

// The drawer closes after a save, taking the button just clicked with it:
// focus lands on what happened.
async function save(draft: CustomerDraft): Promise<void> {
  if (!(await store.save(draft))) return
  if (routeCustomerId() !== null) await router.push({ name: 'customers' })
  await nextTick()
  status.value?.focus()
}

const noSearch = (): boolean => query.value.trim() === ''
</script>

<template>
  <section class="clientes">
    <h1>Clientes</h1>
    <p
      v-if="notice"
      ref="status"
      role="status"
      tabindex="-1"
    >
      {{ notice }}
    </p>
    <p
      v-if="error"
      role="alert"
    >
      {{ error }}
    </p>
    <BaseButton
      v-if="loadFailed"
      @click="store.open()"
    >
      Tentar de novo
    </BaseButton>

    <CustomersToolbar
      v-model="query"
      :can-write="canWrite"
      @search="store.search()"
      @new-customer="store.openNew()"
    />

    <CustomerTable
      v-if="items.length > 0"
      :items="items"
      :time-zone="timeZone"
    />
    <p
      v-else-if="(loading || !loaded) && !error"
      class="muted"
    >
      Carregando os clientes…
    </p>
    <p
      v-else-if="!error"
      class="muted"
    >
      {{ noSearch() ? 'Nenhum cliente ainda.' : 'Nenhum cliente encontrado.' }}
    </p>
    <BaseButton
      v-if="hasMore"
      :disabled="loading"
      @click="store.loadMore()"
    >
      Carregar mais
    </BaseButton>

    <CustomerForm
      v-if="drawerOpen"
      :key="drawerKey"
      :customer="editing"
      :can-write="canWrite"
      :saving="saving"
      :form-error="formError"
      :field-errors="fieldErrors"
      :conflict="conflict"
      :orders="orders"
      :orders-loading="ordersLoading"
      :orders-error="ordersError"
      :orders-capped="ordersCapped"
      @save="save"
      @cancel="close"
      @reload="store.reloadCustomer()"
    />
  </section>
</template>

<style scoped>
.clientes {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 16px;
}
h1,
p {
  margin: 0;
}
[role='alert'],
[role='status'] {
  font-weight: 600;
}
.muted {
  color: var(--enc-ink-secondary);
}
</style>
