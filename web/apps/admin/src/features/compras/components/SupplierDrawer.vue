<script setup lang="ts">
/**
 * SupplierDrawer is "Novo fornecedor" or "Fornecedor": "Nome" and "WhatsApp"
 * (typed with DDD; the store sends the digits) and, for a supplier already
 * saved, "Itens deste fornecedor" with the role, the lead time and, when the
 * API sent it (the owner only), "Custo". A full-screen sheet on the phone,
 * with "Salvar" and "Cancelar" in its sticky footer. A reader without
 * `catalog:write` sees the supplier as text and a "Fechar".
 */
import type { Supplier, SupplierItemUse } from '@encomendo/api'
import { BaseButton, BaseDialog, BaseInput } from '@encomendo/ui'
import { computed, ref } from 'vue'

import { formatMoney } from '../../../format'
import { formatWhatsapp } from '../../../phone'

const props = defineProps<{
  /** supplier is the one being edited, or null for a new one. */
  supplier: Supplier | null
  items: readonly SupplierItemUse[]
  itemsLoading: boolean
  itemsError: string | null
  canWrite: boolean
  saving: boolean
  formError: string | null
  fieldErrors: Readonly<Record<string, string>>
  /** conflict is true after `version_conflict`: the drawer offers "Recarregar". */
  conflict: boolean
}>()

const emit = defineEmits<{
  save: [name: string, whatsapp: string]
  cancel: []
  reload: []
}>()

const name = ref(props.supplier?.name ?? '')
const whatsapp = ref(props.supplier === null ? '' : formatWhatsapp(props.supplier.whatsappPhone))
const showCost = computed(() => props.items.some((item) => item.cost !== undefined))
</script>

<template>
  <BaseDialog
    :title="supplier === null ? 'Novo fornecedor' : 'Fornecedor'"
    variant="drawer"
    @close="emit('cancel')"
  >
    <form
      class="form"
      novalidate
      @submit.prevent="emit('save', name, whatsapp)"
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
        v-model="name"
        label="Nome"
        :disabled="!canWrite"
        :error="fieldErrors.name ?? null"
      />
      <BaseInput
        v-model="whatsapp"
        label="WhatsApp"
        hint="Com DDD, como (92) 99123-4567."
        inputmode="tel"
        autocomplete="tel"
        :disabled="!canWrite"
        :error="fieldErrors.whatsappPhone ?? null"
      />

      <section
        v-if="supplier !== null"
        class="items"
        aria-labelledby="supplier-items-heading"
      >
        <h3 id="supplier-items-heading">
          Itens deste fornecedor
        </h3>
        <p
          v-if="itemsError"
          role="alert"
        >
          {{ itemsError }}
        </p>
        <p
          v-else-if="itemsLoading"
          class="muted"
        >
          Carregando os itens…
        </p>
        <p
          v-else-if="items.length === 0"
          class="muted"
        >
          Nenhum item usa este fornecedor.
        </p>
        <table
          v-else
          class="enc-cards"
        >
          <caption class="enc-visually-hidden">
            Itens deste fornecedor
          </caption>
          <thead>
            <tr>
              <th scope="col">
                Item
              </th>
              <th scope="col">
                Papel
              </th>
              <th scope="col">
                Prazo (dias)
              </th>
              <th
                v-if="showCost"
                scope="col"
              >
                Custo
              </th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="item in items"
              :key="`${item.itemId}-${item.role}`"
            >
              <th scope="row">
                {{ item.itemName }}
              </th>
              <td data-label="Papel">
                {{ item.role === 'main' ? 'Principal' : 'Reserva' }}
              </td>
              <td
                data-label="Prazo (dias)"
                class="enc-number"
              >
                {{ item.leadDays }}
              </td>
              <td
                v-if="showCost"
                data-label="Custo"
                class="enc-number"
              >
                {{ item.cost ? formatMoney(item.cost.cents, item.cost.currency) : '—' }}
              </td>
            </tr>
          </tbody>
        </table>
      </section>

      <div class="buttons enc-sheet-actions">
        <BaseButton
          v-if="canWrite"
          variant="primary"
          type="submit"
          :disabled="saving"
        >
          Salvar
        </BaseButton>
        <BaseButton @click="emit('cancel')">
          {{ canWrite ? 'Cancelar' : 'Fechar' }}
        </BaseButton>
      </div>
    </form>
  </BaseDialog>
</template>

<style scoped>
.form {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
p,
h3 {
  margin: 0;
}
h3 {
  font-size: 15px;
}
.alert {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 12px;
}
[role='alert'] {
  color: var(--enc-status-alert-text);
  font-weight: 600;
}
.muted {
  color: var(--enc-ink-secondary);
}
.items {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
table {
  width: 100%;
  border-collapse: collapse;
  font-family: var(--enc-font-condensed);
  font-size: var(--enc-table-size, 13px);
}
th,
td {
  padding: 8px 12px;
  border-bottom: 1px solid var(--enc-hairline);
  text-align: left;
}
thead th {
  color: var(--enc-ink-secondary);
}
tbody th {
  font-weight: 400;
}
.buttons {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
</style>
