<script setup lang="ts">
/**
 * ItemDrawer is "Novo item" / "Editar item": the whole item form. It holds
 * the draft and hands it to the store on "Salvar". Roles without
 * `catalog:write` see the same fields, disabled, with no buttons that change
 * anything. The Custo fields show only to the owner (ADR 0025).
 */
import type { Attribute, Category, Item, Photo, Supplier } from '@encomendo/api'
import { BaseButton, BaseDialog, BaseInput, BaseSelect } from '@encomendo/ui'
import { computed, reactive, ref, useId } from 'vue'

import { draftFromItem, emptyDraft, emptySupplier, type ItemDraft } from '../itemForm'
import { isSourcingMode, sourcingOptions } from '../labels'
import OptionsEditor from './OptionsEditor.vue'
import PhotosEditor from './PhotosEditor.vue'
import PublishedSwitch from './PublishedSwitch.vue'
import SupplierFields from './SupplierFields.vue'

const props = defineProps<{
  /** item is the item being edited, or null for a new one. */
  item: Item | null
  canWrite: boolean
  /** showCosts is true for the owner only. */
  showCosts: boolean
  suppliers: readonly Supplier[]
  categories: readonly Category[]
  attributes: readonly Attribute[]
  stockEnabled: boolean
  productionEnabled: boolean
  saving: boolean
  /** formError is the message above the form; `fieldErrors` the ones next to the fields. */
  formError: string | null
  fieldErrors: Readonly<Record<string, string>>
  /** conflict offers "Recarregar" beside the message. */
  conflict: boolean
  photos: readonly Photo[]
  photoError: string | null
  photoBusy: boolean
  addingSupplier: boolean
  supplierError: string | null
  supplierFieldErrors: Readonly<Record<string, string>>
}>()

const emit = defineEmits<{
  save: [draft: ItemDraft]
  cancel: []
  archive: []
  restore: []
  reload: []
  createSupplier: [name: string, whatsapp: string]
  cancelNewSupplier: []
  addPhotos: [files: File[]]
  coverPhoto: [photo: Photo]
  removePhoto: [photo: Photo]
}>()

const draft = reactive<ItemDraft>(props.item === null ? emptyDraft() : draftFromItem(props.item))
/** newSupplierFor is the slot showing the "Novo fornecedor" form. */
const newSupplierFor = ref<'main' | 'backup' | null>(null)
const descriptionId = useId()

const archived = computed(() => props.item?.archived === true)
const pickable = computed(() => props.attributes.filter((attribute) => attribute.values.length > 0))
const title = computed(() => (!props.canWrite ? 'Item' : props.item === null ? 'Novo item' : 'Editar item'))
const modeOptions = computed(() => sourcingOptions(props.stockEnabled, props.productionEnabled, draft.sourcingMode))
const categoryOptions = computed(() => [{ value: '', label: 'Sem categoria' }, ...props.categories.map((category) => ({ value: category.id, label: category.name }))])
const inHouse = computed(() => draft.sourcingMode === 'in_house')

function supplierErrors(slot: 'main' | 'backup'): Record<string, string> {
  const errors = props.fieldErrors
  return {
    ...(errors[`${slot}Supplier`] === undefined ? {} : { supplier: errors[`${slot}Supplier`] as string }),
    ...(errors[`${slot}LeadDays`] === undefined ? {} : { leadDays: errors[`${slot}LeadDays`] as string }),
    ...(errors[`${slot}Cost`] === undefined ? {} : { cost: errors[`${slot}Cost`] as string }),
  }
}

function chooseMode(value: string): void {
  if (isSourcingMode(value)) draft.sourcingMode = value
}

function pick(valueId: string, event: Event): void {
  if (!(event.target instanceof HTMLInputElement)) return
  const others = draft.attributeValueIds.filter((id) => id !== valueId)
  draft.attributeValueIds = event.target.checked ? [...others, valueId] : others
}

function addBackup(): void {
  draft.backup = emptySupplier()
}

function removeBackup(): void {
  draft.backup = null
  if (newSupplierFor.value === 'backup') newSupplierFor.value = null
}

function startNew(slot: 'main' | 'backup'): void {
  newSupplierFor.value = slot
}

function cancelNew(slot: 'main' | 'backup'): void {
  if (newSupplierFor.value === slot) newSupplierFor.value = null
  emit('cancelNewSupplier')
}

defineExpose({
  /** supplierCreated picks a supplier "Novo fornecedor" just made, in the slot that asked, and closes the form. */
  supplierCreated(supplierId: string): void {
    const slot = newSupplierFor.value
    if (slot === null) return
    const target = slot === 'main' ? draft.main : draft.backup
    if (target !== null) {
      target.supplierId = supplierId
      target.cost = ''
    }
    newSupplierFor.value = null
  },
})
</script>

<template>
  <BaseDialog
    :title="title"
    variant="drawer"
    @close="emit('cancel')"
  >
    <form
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

      <fieldset
        class="fields"
        :disabled="!canWrite"
      >
        <BaseInput
          v-model="draft.name"
          label="Nome"
          :error="fieldErrors.name ?? null"
        />
        <div class="field">
          <label :for="descriptionId">Descrição (opcional)</label>
          <textarea
            :id="descriptionId"
            v-model="draft.description"
            rows="3"
          />
        </div>
        <BaseSelect
          v-model="draft.categoryId"
          label="Categoria"
          :options="categoryOptions"
          :error="fieldErrors.categoryId ?? null"
        />
        <BaseSelect
          :model-value="draft.sourcingMode"
          label="Origem"
          :options="modeOptions"
          :error="fieldErrors.sourcingMode ?? null"
          @update:model-value="chooseMode"
        />
        <BaseInput
          v-model="draft.price"
          label="Preço"
          inputmode="decimal"
          :error="fieldErrors.price ?? null"
        />

        <template v-if="inHouse">
          <BaseInput
            v-model="draft.productionDays"
            label="Dias de produção"
            inputmode="numeric"
            :error="fieldErrors.productionDays ?? null"
          />
          <BaseInput
            v-if="showCosts"
            v-model="draft.productionCost"
            label="Custo de produção"
            inputmode="decimal"
            :error="fieldErrors.productionCost ?? null"
          />
        </template>
        <template v-else>
          <SupplierFields
            v-model="draft.main"
            legend="Fornecedor"
            supplier-label="Fornecedor principal"
            :suppliers="suppliers"
            :show-cost="showCosts"
            :stored-costs="draft.stored.supplierCosts"
            :errors="supplierErrors('main')"
            :adding="newSupplierFor === 'main'"
            :creating="addingSupplier"
            :create-error="newSupplierFor === 'main' ? supplierError : null"
            :create-field-errors="newSupplierFor === 'main' ? supplierFieldErrors : {}"
            @start-new="startNew('main')"
            @cancel-new="cancelNew('main')"
            @create-supplier="(name, whatsapp) => emit('createSupplier', name, whatsapp)"
          />
          <SupplierFields
            v-if="draft.backup !== null"
            v-model="draft.backup"
            legend="Fornecedor reserva"
            supplier-label="Fornecedor reserva"
            :suppliers="suppliers"
            :show-cost="showCosts"
            :stored-costs="draft.stored.supplierCosts"
            :errors="supplierErrors('backup')"
            :adding="newSupplierFor === 'backup'"
            :creating="addingSupplier"
            :create-error="newSupplierFor === 'backup' ? supplierError : null"
            :create-field-errors="newSupplierFor === 'backup' ? supplierFieldErrors : {}"
            @start-new="startNew('backup')"
            @cancel-new="cancelNew('backup')"
            @create-supplier="(name, whatsapp) => emit('createSupplier', name, whatsapp)"
          >
            <div>
              <BaseButton @click="removeBackup">
                Remover fornecedor reserva
              </BaseButton>
            </div>
          </SupplierFields>
          <div v-else-if="canWrite">
            <BaseButton @click="addBackup">
              Adicionar fornecedor reserva
            </BaseButton>
          </div>
        </template>

        <fieldset
          v-for="attribute in pickable"
          :key="attribute.id"
          class="attribute"
        >
          <legend>{{ attribute.name }}</legend>
          <label
            v-for="value in attribute.values"
            :key="value.id"
            class="check"
          >
            <input
              type="checkbox"
              :checked="draft.attributeValueIds.includes(value.id)"
              @change="(event) => pick(value.id, event)"
            >
            {{ value.label }}
          </label>
        </fieldset>

        <OptionsEditor
          v-model="draft.options"
          :can-write="canWrite"
          :errors="fieldErrors"
        />
      </fieldset>

      <PhotosEditor
        :photos="photos"
        :saved="item !== null"
        :can-write="canWrite"
        :busy="photoBusy"
        :error="photoError"
        @add="(files) => emit('addPhotos', files)"
        @cover="(photo) => emit('coverPhoto', photo)"
        @remove="(photo) => emit('removePhoto', photo)"
      />

      <div class="check">
        <PublishedSwitch
          :checked="draft.published && !archived"
          label="Mostrar na página da loja"
          :disabled="!canWrite || archived"
          @toggle="(published) => (draft.published = published)"
        />
        <span aria-hidden="true">Mostrar na página da loja</span>
      </div>

      <div class="buttons">
        <template v-if="canWrite">
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
          <BaseButton
            v-if="item !== null && !archived"
            @click="emit('archive')"
          >
            Arquivar
          </BaseButton>
          <BaseButton
            v-if="item !== null && archived"
            @click="emit('restore')"
          >
            Restaurar
          </BaseButton>
        </template>
        <BaseButton
          v-else
          @click="emit('cancel')"
        >
          Fechar
        </BaseButton>
      </div>
    </form>
  </BaseDialog>
</template>

<style scoped>
.form,
.fields {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.fields {
  min-width: 0;
  margin: 0;
  padding: 0;
  border: 0;
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
.attribute {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin: 0;
  padding: 12px;
  border: 1px solid var(--enc-hairline);
  border-radius: var(--enc-radius-card);
}
legend {
  padding: 0 4px;
  font-weight: 600;
}
.check {
  display: flex;
  align-items: center;
  gap: 8px;
}
.buttons {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
</style>
