<script setup lang="ts">
/**
 * AddressFields is one address of the customer form: "Nome do endereço"
 * (such as "Casa"), CEP, street, number, complement, district, city, UF and
 * "Remover endereço". Errors come keyed the way the API names them,
 * "addresses[2].cep", and this component reads its own index.
 */
import { BaseButton, BaseInput, BaseSelect } from '@encomendo/ui'
import { computed } from 'vue'

import { type AddressDraft, addressField, isUF, ufs } from '../customerForm'

const props = defineProps<{
  /** index is the address's place in the customer's list, from 0. */
  index: number
  errors: Readonly<Record<string, string>>
}>()

const address = defineModel<AddressDraft>({ required: true })

const emit = defineEmits<{
  remove: []
}>()

const stateOptions = [{ value: '', label: 'Escolha a UF' }, ...ufs.map((uf) => ({ value: uf, label: uf }))]

function error(field: keyof AddressDraft): string | null {
  return props.errors[addressField(props.index, field)] ?? null
}

const legend = computed(() => (address.value.label.trim() === '' ? `Endereço ${props.index + 1}` : address.value.label.trim()))

function chooseState(value: string): void {
  if (value === '') address.value = { ...address.value, state: '' }
  else if (isUF(value)) address.value = { ...address.value, state: value }
}

function set(field: keyof AddressDraft, value: string): void {
  address.value = { ...address.value, [field]: value }
}
</script>

<template>
  <fieldset class="address">
    <legend>{{ legend }}</legend>
    <div class="grid">
      <BaseInput
        :model-value="address.label"
        label="Nome do endereço"
        :error="error('label')"
        @update:model-value="set('label', $event)"
      />
      <BaseInput
        :model-value="address.cep"
        label="CEP"
        inputmode="numeric"
        autocomplete="postal-code"
        :error="error('cep')"
        @update:model-value="set('cep', $event)"
      />
      <div class="wide">
        <BaseInput
          :model-value="address.street"
          label="Rua"
          autocomplete="address-line1"
          :error="error('street')"
          @update:model-value="set('street', $event)"
        />
      </div>
      <BaseInput
        :model-value="address.number"
        label="Número"
        :error="error('number')"
        @update:model-value="set('number', $event)"
      />
      <BaseInput
        :model-value="address.complement"
        label="Complemento"
        :error="error('complement')"
        @update:model-value="set('complement', $event)"
      />
      <BaseInput
        :model-value="address.district"
        label="Bairro"
        :error="error('district')"
        @update:model-value="set('district', $event)"
      />
      <BaseInput
        :model-value="address.city"
        label="Cidade"
        autocomplete="address-level2"
        :error="error('city')"
        @update:model-value="set('city', $event)"
      />
      <BaseSelect
        :model-value="address.state"
        label="UF"
        :options="stateOptions"
        :error="error('state')"
        @update:model-value="chooseState"
      />
    </div>
    <div>
      <BaseButton @click="emit('remove')">
        Remover endereço
      </BaseButton>
    </div>
  </fieldset>
</template>

<style scoped>
.address {
  display: flex;
  flex-direction: column;
  gap: 12px;
  min-width: 0;
  margin: 0;
  padding: 12px;
  border: 1px solid var(--enc-hairline);
  border-radius: var(--enc-radius-card);
}
legend {
  padding: 0 4px;
  font-weight: 600;
}
.grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}
.wide {
  grid-column: 1 / -1;
}

/* Phone: one field per row. */
@media (max-width: 640px) {
  .grid {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
