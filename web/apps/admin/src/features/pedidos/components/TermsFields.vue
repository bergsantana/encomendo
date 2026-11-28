<script setup lang="ts">
/**
 * TermsFields are "Entrega" and "Valores" of the order form. "Entrega" is
 * "Retirada na loja" or "Entrega"; a delivery shows "Endereço" (the
 * customer's, or "Novo endereço", which is saved on the customer with the
 * order) and
 * "Frete". "Valores" (discount, deposit percent, days) is only for roles with
 * `order:manage`; the store's defaults are the hints, and an empty field
 * means "use the default".
 */
import type { Address } from '@encomendo/api'
import { BaseInput, BaseSelect } from '@encomendo/ui'
import { computed } from 'vue'

import AddressFields from '../../clientes/components/AddressFields.vue'
import { niche } from '../../../niche'
import { newAddressChoice, type TermsDraft } from '../orderForm'

const props = defineProps<{
  /** addresses are the customer's own, to choose from; a customer made with the order has none. */
  addresses: readonly Address[]
  canManage: boolean
  /** defaults are the store's deposit percent and days, for the hints. */
  defaults: { depositPercent: number; assemblyDays: number; safetyMarginDays: number } | null
  /** errors are the messages by field; the new address's are `addresses[0].…`, and `addresses` is the limit of 10. */
  errors: Readonly<Record<string, string>>
}>()

const terms = defineModel<TermsDraft>({ required: true })

function set<K extends keyof TermsDraft>(field: K, value: TermsDraft[K]): void {
  terms.value = { ...terms.value, [field]: value }
}

const addressOptions = computed(() => [
  { value: '', label: 'Escolha o endereço' },
  ...props.addresses.map((address) => ({ value: address.id, label: `${address.label}: ${address.street}, ${address.number}` })),
  { value: newAddressChoice, label: 'Novo endereço' },
])

function hintDays(days: number | undefined): string | null {
  return days === undefined ? null : `Padrão da loja: ${days} dias`
}
</script>

<template>
  <div class="terms">
    <fieldset>
      <legend>Entrega</legend>
      <label class="enc-check">
        <input
          type="radio"
          name="handover"
          value="pickup"
          :checked="terms.handover === 'pickup'"
          @change="set('handover', 'pickup')"
        >
        Retirada na loja
      </label>
      <label class="enc-check">
        <input
          type="radio"
          name="handover"
          value="delivery"
          :checked="terms.handover === 'delivery'"
          @change="set('handover', 'delivery')"
        >
        Entrega
      </label>

      <template v-if="terms.handover === 'delivery'">
        <BaseSelect
          :model-value="terms.addressId"
          label="Endereço"
          :options="addressOptions"
          :error="errors.address ?? null"
          @update:model-value="set('addressId', $event)"
        />
        <div
          v-if="terms.addressId === newAddressChoice"
          class="new-address"
        >
          <p
            v-if="errors.addresses"
            role="alert"
            class="error"
          >
            {{ errors.addresses }}
          </p>
          <AddressFields
            :model-value="terms.newAddress"
            :index="0"
            :errors="errors"
            @update:model-value="set('newAddress', $event)"
            @remove="set('addressId', '')"
          />
        </div>
        <BaseInput
          :model-value="terms.freight"
          label="Frete"
          inputmode="decimal"
          :error="errors.freight ?? null"
          @update:model-value="set('freight', $event)"
        />
      </template>
    </fieldset>

    <fieldset v-if="canManage">
      <legend>Valores</legend>
      <BaseInput
        :model-value="terms.discount"
        label="Desconto"
        inputmode="decimal"
        :error="errors.discount ?? null"
        @update:model-value="set('discount', $event)"
      />
      <BaseInput
        :model-value="terms.depositPercent"
        label="Sinal (%)"
        inputmode="numeric"
        :hint="defaults === null ? null : `Padrão da loja: ${defaults.depositPercent}%`"
        :error="errors.depositPercent ?? null"
        @update:model-value="set('depositPercent', $event)"
      />
      <BaseInput
        :model-value="terms.assemblyDays"
        :label="`Dias de ${niche.montagem}`"
        inputmode="numeric"
        :hint="hintDays(defaults?.assemblyDays)"
        :error="errors.assemblyDays ?? null"
        @update:model-value="set('assemblyDays', $event)"
      />
      <BaseInput
        :model-value="terms.safetyMarginDays"
        label="Margem de segurança (dias)"
        inputmode="numeric"
        :hint="hintDays(defaults?.safetyMarginDays)"
        :error="errors.safetyMarginDays ?? null"
        @update:model-value="set('safetyMarginDays', $event)"
      />
    </fieldset>
  </div>
</template>

<style scoped>
.terms {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
fieldset {
  display: flex;
  flex-direction: column;
  gap: 12px;
  max-width: 420px;
  min-width: 0;
  margin: 0;
  padding: 0;
  border: 0;
}
legend {
  padding: 0;
  font-weight: 600;
}
.new-address {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.error {
  margin: 0;
  color: var(--enc-status-alert-text);
  font-weight: 600;
}

@media (max-width: 640px) {
  fieldset {
    max-width: none;
  }
}
</style>
