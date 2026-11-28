<script setup lang="ts">
/**
 * CustomerForm is "Novo cliente" / "Cliente": the customer drawer (a full
 * screen sheet on the phone). Roles with `customer:write` get the form with
 * its addresses; the others get the same customer as labelled lines and
 * "Fechar". The customer's orders show for both. The form holds the draft and
 * hands it to the store on "Salvar".
 */
import type { Customer, OrderSummary } from '@encomendo/api'
import { BaseButton, BaseDialog, BaseInput } from '@encomendo/ui'
import { reactive, useId } from 'vue'

import { type CustomerDraft, draftOf, emptyAddress, maxAddresses } from '../customerForm'
import AddressFields from './AddressFields.vue'
import CustomerDetails from './CustomerDetails.vue'
import CustomerOrders from './CustomerOrders.vue'

const props = defineProps<{
  /** customer is the customer being edited, or null for a new one. */
  customer: Customer | null
  canWrite: boolean
  saving: boolean
  /** formError is the message above the form; `fieldErrors` the ones next to the fields. */
  formError: string | null
  fieldErrors: Readonly<Record<string, string>>
  /** conflict offers "Recarregar" beside the message. */
  conflict: boolean
  orders: readonly OrderSummary[]
  ordersLoading: boolean
  ordersError: string | null
  ordersCapped: boolean
}>()

const emit = defineEmits<{
  save: [draft: CustomerDraft]
  cancel: []
  reload: []
}>()

const draft = reactive<CustomerDraft>(draftOf(props.customer))
const notesId = useId()

function addAddress(): void {
  draft.addresses.push(emptyAddress())
}

function removeAddress(index: number): void {
  draft.addresses.splice(index, 1)
}
</script>

<template>
  <BaseDialog
    :title="!canWrite ? 'Cliente' : customer === null ? 'Novo cliente' : 'Cliente'"
    variant="drawer"
    @close="emit('cancel')"
  >
    <form
      v-if="canWrite"
      class="form"
      novalidate
      @submit.prevent="emit('save', draft)"
    >
      <div
        v-if="formError"
        class="alert"
        role="alert"
      >
        <p>{{ formError }}</p>
        <BaseButton
          v-if="conflict"
          @click="emit('reload')"
        >
          Recarregar
        </BaseButton>
      </div>

      <BaseInput
        v-model="draft.name"
        label="Nome"
        autocomplete="off"
        :error="fieldErrors.name ?? null"
      />
      <BaseInput
        v-model="draft.phone"
        label="WhatsApp"
        hint="Com DDD, como (92) 99123-4567."
        inputmode="tel"
        autocomplete="off"
        :error="fieldErrors.phone ?? null"
      />
      <BaseInput
        v-model="draft.email"
        label="E-mail"
        type="email"
        :error="fieldErrors.email ?? null"
      />
      <div class="field">
        <label :for="notesId">Observações</label>
        <textarea
          :id="notesId"
          v-model="draft.notes"
          rows="3"
        />
        <p
          v-if="fieldErrors.notes"
          class="error"
        >
          {{ fieldErrors.notes }}
        </p>
      </div>

      <fieldset class="addresses">
        <legend>Endereços</legend>
        <p
          v-if="fieldErrors.addresses"
          class="error"
        >
          {{ fieldErrors.addresses }}
        </p>
        <AddressFields
          v-for="(address, index) in draft.addresses"
          :key="address.id ?? `new-${index}`"
          :model-value="address"
          :index="index"
          :errors="fieldErrors"
          @update:model-value="draft.addresses[index] = $event"
          @remove="removeAddress(index)"
        />
        <div>
          <BaseButton
            :disabled="draft.addresses.length >= maxAddresses"
            @click="addAddress"
          >
            Adicionar endereço
          </BaseButton>
        </div>
      </fieldset>

      <CustomerOrders
        v-if="customer !== null"
        :orders="orders"
        :loading="ordersLoading"
        :error="ordersError"
        :capped="ordersCapped"
      />

      <div class="buttons enc-sheet-actions">
        <BaseButton
          variant="primary"
          type="submit"
          :disabled="saving"
        >
          Salvar
        </BaseButton>
        <BaseButton @click="emit('cancel')">
          Cancelar
        </BaseButton>
      </div>
    </form>

    <div
      v-else-if="customer !== null"
      class="form"
    >
      <CustomerDetails :customer="customer" />
      <CustomerOrders
        :orders="orders"
        :loading="ordersLoading"
        :error="ordersError"
        :capped="ordersCapped"
      />
      <div class="buttons enc-sheet-actions">
        <BaseButton @click="emit('cancel')">
          Fechar
        </BaseButton>
      </div>
    </div>
  </BaseDialog>
</template>

<style scoped>
.form {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.field {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.field label {
  font-weight: 600;
}
textarea {
  padding: 6px 10px;
  border: 1px solid var(--enc-secondary-border);
  border-radius: var(--enc-radius-card);
  background: var(--enc-surface);
  color: var(--enc-ink);
  font: inherit;
}
.addresses {
  display: flex;
  flex-direction: column;
  gap: 12px;
  min-width: 0;
  margin: 0;
  padding: 0;
  border: 0;
}
legend {
  padding: 0;
  font-weight: 600;
}
.error {
  margin: 0;
  color: var(--enc-status-alert-text);
}
.alert {
  display: flex;
  align-items: center;
  gap: 12px;
  font-weight: 600;
  color: var(--enc-status-alert-text);
}
.alert p {
  margin: 0;
}
.buttons {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

@media (max-width: 640px) {
  .alert {
    flex-wrap: wrap;
  }
}
</style>
