<script setup lang="ts">
/**
 * CatalogoView is the Catálogo screen. Only the Itens tab exists so far; the
 * Kits tab joins it in step 3.13 as one more entry in `tabs` and one more
 * panel. Every role reads it (`catalog:read`); only owner and admin
 * (`catalog:write`) get the controls that change anything, and only the owner
 * gets cost and margin, because the API sends them to no one else.
 */
import type { Attribute, Category, Photo } from '@encomendo/api'
import { BaseButton } from '@encomendo/ui'
import { storeToRefs } from 'pinia'
import { nextTick, onMounted, ref, useTemplateRef } from 'vue'

import { usePermission } from '../../../composables/usePermission'
import AttributesDialog from '../components/AttributesDialog.vue'
import CategoriesDialog from '../components/CategoriesDialog.vue'
import ItemDrawer from '../components/ItemDrawer.vue'
import ItemsTable from '../components/ItemsTable.vue'
import ItemsToolbar from '../components/ItemsToolbar.vue'
import type { ItemDraft } from '../itemForm'
import { type ItemFilters, useCatalogStore } from '../store'

const store = useCatalogStore()
const {
  items,
  hasMore,
  loading,
  loaded,
  filters,
  suppliers,
  stockEnabled,
  productionEnabled,
  sendsCosts,
  notice,
  error,
  editing,
  drawerOpen,
  drawerKey,
  photos,
  photoError,
  photoBusy,
  conflict,
  saving,
  formError,
  fieldErrors,
  addingSupplier,
  supplierError,
  supplierFieldErrors,
  categories,
  attributes,
  categoryError,
  categoryFormError,
  categoryFieldErrors,
  attributeFormError,
  attributeFieldErrors,
  attributeFormId,
  attributeSaving,
} = storeToRefs(store)
const canWrite = usePermission('catalog:write')

/** tabs are the sections of Catálogo. Step 3.13 adds Kits. */
const tabs = [{ id: 'itens', label: 'Itens' }]
const tab = ref('itens')
const dialog = ref<'categories' | 'attributes' | null>(null)
const status = useTemplateRef<HTMLElement>('status')
const drawer = useTemplateRef<InstanceType<typeof ItemDrawer>>('drawer')
const categoriesDialog = useTemplateRef<InstanceType<typeof CategoriesDialog>>('categoriesDialog')
const attributesDialog = useTemplateRef<InstanceType<typeof AttributesDialog>>('attributesDialog')

onMounted(() => store.open())

const noFilters = (): boolean => filters.value.q.trim() === '' && filters.value.categoryId === '' && !filters.value.includeArchived

function applyFilters(patch: Partial<ItemFilters>): void {
  Object.assign(store.filters, patch)
  void store.search()
}

// The drawer closes after a save, taking the button just clicked with it:
// focus lands on what happened.
async function focusStatus(): Promise<void> {
  await nextTick()
  status.value?.focus()
}

async function save(draft: ItemDraft): Promise<void> {
  if (await store.saveItem(draft)) await focusStatus()
}

async function archive(): Promise<void> {
  await store.archive()
  if (notice.value !== null) await focusStatus()
}

async function restore(): Promise<void> {
  await store.restore()
  if (notice.value !== null) await focusStatus()
}

async function createSupplier(name: string, whatsapp: string): Promise<void> {
  const supplier = await store.addSupplier(name, whatsapp)
  if (supplier !== null) drawer.value?.supplierCreated(supplier.id)
}

async function addCategory(name: string): Promise<void> {
  if (await store.addCategory(name)) categoriesDialog.value?.added()
}

async function renameCategory(category: Category, name: string): Promise<void> {
  if (await store.renameCategory(category, name)) categoriesDialog.value?.renamedDone()
}

async function saveAttribute(attribute: Attribute | null, draft: Parameters<typeof store.saveAttribute>[1]): Promise<void> {
  if ((await store.saveAttribute(attribute, draft)) && attribute === null) attributesDialog.value?.saved()
}

function coverPhoto(photo: Photo): void {
  void store.makeCover(photo)
}
</script>

<template>
  <section class="catalogo">
    <h1>Catálogo</h1>
    <div
      class="tabs"
      role="tablist"
      aria-label="Catálogo"
    >
      <button
        v-for="candidate in tabs"
        :id="`tab-${candidate.id}`"
        :key="candidate.id"
        type="button"
        role="tab"
        :aria-selected="tab === candidate.id"
        :aria-controls="`panel-${candidate.id}`"
        @click="tab = candidate.id"
      >
        {{ candidate.label }}
      </button>
    </div>

    <div
      v-if="tab === 'itens'"
      id="panel-itens"
      class="panel"
      role="tabpanel"
      aria-labelledby="tab-itens"
    >
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
        v-if="error && !loaded"
        @click="store.open()"
      >
        Tentar de novo
      </BaseButton>

      <ItemsToolbar
        :filters="filters"
        :categories="categories"
        :can-write="canWrite"
        @change="applyFilters"
        @new-item="store.openNew()"
        @categories="dialog = 'categories'"
        @attributes="dialog = 'attributes'"
      />

      <ItemsTable
        v-if="items.length > 0"
        :items="items"
        :categories="categories"
        :can-write="canWrite"
        @edit="store.openEdit($event)"
        @publish="(item, published) => store.setPublished(item, published)"
      />
      <p
        v-else-if="loading || !loaded"
        class="muted"
      >
        Carregando o catálogo…
      </p>
      <template v-else-if="!error">
        <p class="muted">
          {{ noFilters() ? 'Nenhum item no catálogo ainda.' : 'Nenhum item encontrado.' }}
        </p>
        <BaseButton
          v-if="noFilters() && canWrite"
          variant="primary"
          @click="store.openNew()"
        >
          Novo item
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

    <ItemDrawer
      v-if="drawerOpen"
      ref="drawer"
      :key="drawerKey"
      :item="editing"
      :can-write="canWrite"
      :show-costs="sendsCosts"
      :suppliers="suppliers"
      :categories="categories"
      :attributes="attributes"
      :stock-enabled="stockEnabled"
      :production-enabled="productionEnabled"
      :saving="saving"
      :form-error="formError"
      :field-errors="fieldErrors"
      :conflict="conflict"
      :photos="photos"
      :photo-error="photoError"
      :photo-busy="photoBusy"
      :adding-supplier="addingSupplier"
      :supplier-error="supplierError"
      :supplier-field-errors="supplierFieldErrors"
      @save="save"
      @cancel="store.closeDrawer()"
      @archive="archive"
      @restore="restore"
      @reload="store.reloadItem()"
      @create-supplier="createSupplier"
      @cancel-new-supplier="store.clearSupplierForm()"
      @add-photos="store.addPhotos($event)"
      @cover-photo="coverPhoto"
      @remove-photo="store.removePhoto($event)"
    />
    <CategoriesDialog
      v-if="dialog === 'categories'"
      ref="categoriesDialog"
      :categories="categories"
      :can-write="canWrite"
      :error="categoryError"
      :form-error="categoryFormError"
      :field-errors="categoryFieldErrors"
      @close="dialog = null"
      @add="addCategory"
      @rename="renameCategory"
      @move="(category, position) => store.moveCategory(category, position)"
      @remove="store.removeCategory($event)"
      @clear-errors="store.clearCategoryForm()"
    />
    <AttributesDialog
      v-if="dialog === 'attributes'"
      ref="attributesDialog"
      :attributes="attributes"
      :can-write="canWrite"
      :active-id="attributeFormId"
      :error="attributeFormError"
      :field-errors="attributeFieldErrors"
      :saving="attributeSaving"
      @close="dialog = null"
      @save="saveAttribute"
      @remove="store.removeAttribute($event)"
      @clear-errors="store.clearAttributeForm()"
    />
  </section>
</template>

<style scoped>
.catalogo {
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
h1 {
  margin: 0;
}
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
.tabs {
  display: flex;
  gap: 4px;
  border-bottom: 1px solid var(--enc-hairline);
  width: 100%;
}
.tabs [role='tab'] {
  padding: 8px 14px;
  border: 0;
  background: none;
  color: var(--enc-ink);
  font: inherit;
  cursor: pointer;
}
.tabs [role='tab'][aria-selected='true'] {
  box-shadow: inset 0 -3px 0 var(--enc-accent);
  font-weight: 600;
}
</style>
