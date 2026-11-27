<script setup lang="ts">
/**
 * KitsTab is the Kits tab of Catálogo: the kit table and the kit editor.
 * Every role reads it (`catalog:read`); only owner and admin
 * (`catalog:write`) get the controls that change anything, and only the owner
 * gets margin, because the API sends it to no one else.
 */
import type { Photo } from '@encomendo/api'
import { BaseButton } from '@encomendo/ui'
import { storeToRefs } from 'pinia'
import { nextTick, onMounted, useId, useTemplateRef } from 'vue'

import { usePermission } from '../../../composables/usePermission'
import KitDrawer from '../components/KitDrawer.vue'
import KitsTable from '../components/KitsTable.vue'
import type { KitDraft } from '../kitForm'
import { useCatalogStore } from '../store'

const store = useCatalogStore()
const {
  kits,
  kitsHasMore: hasMore,
  kitsLoading: loading,
  kitsLoaded: loaded,
  kitsLoadFailed: loadFailed,
  includeArchivedKits: includeArchived,
  catalogItems,
  categories,
  attributes,
  defaultAssemblyDays,
  defaultSafetyMarginDays,
  publishingKitId: publishingId,
  kitNotice: notice,
  kitError: error,
  editingKit: editing,
  kitEditorOpen: editorOpen,
  kitPhotos: photos,
  kitPhotoError: photoError,
  kitPhotoBusy: photoBusy,
  kitConflict: conflict,
  kitSaving: saving,
  kitFormError: formError,
  kitFieldErrors: fieldErrors,
} = storeToRefs(store)
const canWrite = usePermission('catalog:write')
const archivedId = useId()
const status = useTemplateRef<HTMLElement>('status')

onMounted(() => store.openKits())

// The editor closes after an archive, taking the button just clicked with it:
// focus lands on what happened.
async function focusStatus(): Promise<void> {
  await nextTick()
  status.value?.focus()
}

async function save(draft: KitDraft): Promise<void> {
  await store.saveKit(draft)
}

async function archive(): Promise<void> {
  await store.archiveKit()
  if (notice.value !== null) await focusStatus()
}

async function restore(): Promise<void> {
  await store.restoreKit()
  if (notice.value !== null) await focusStatus()
}

function toggleArchived(event: Event): void {
  if (event.target instanceof HTMLInputElement) void store.setIncludeArchivedKits(event.target.checked)
}

function coverPhoto(photo: Photo): void {
  void store.makeKitCover(photo)
}
</script>

<template>
  <div
    id="panel-kits"
    class="panel"
    role="tabpanel"
    aria-labelledby="tab-kits"
  >
    <p
      v-if="notice && !editorOpen"
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
      @click="store.openKits()"
    >
      Tentar de novo
    </BaseButton>

    <div class="toolbar">
      <span class="archived">
        <input
          :id="archivedId"
          type="checkbox"
          :checked="includeArchived"
          @change="toggleArchived"
        >
        <label :for="archivedId">Mostrar arquivados</label>
      </span>
      <BaseButton
        v-if="canWrite"
        variant="primary"
        class="new"
        @click="store.openNewKit()"
      >
        Novo kit
      </BaseButton>
    </div>

    <KitsTable
      v-if="kits.length > 0"
      :kits="kits"
      :can-write="canWrite"
      :publishing-id="publishingId"
      @edit="store.openKit($event)"
      @publish="(kit, published) => store.publishKit(kit, published)"
    />
    <p
      v-else-if="(loading || !loaded) && !error"
      class="muted"
    >
      Carregando os kits…
    </p>
    <template v-else-if="!error">
      <p class="muted">
        Nenhum kit ainda.
      </p>
      <BaseButton
        v-if="canWrite"
        @click="store.openNewKit()"
      >
        Novo kit
      </BaseButton>
    </template>
    <BaseButton
      v-if="hasMore"
      :disabled="loading"
      @click="store.loadMoreKits()"
    >
      Carregar mais
    </BaseButton>

    <KitDrawer
      v-if="editorOpen"
      :kit="editing"
      :can-write="canWrite"
      :catalog-items="catalogItems"
      :categories="categories"
      :attributes="attributes"
      :default-assembly-days="defaultAssemblyDays"
      :default-safety-margin-days="defaultSafetyMarginDays"
      :saving="saving"
      :form-error="formError"
      :field-errors="fieldErrors"
      :conflict="conflict"
      :notice="notice"
      :photos="photos"
      :photo-error="photoError"
      :photo-busy="photoBusy"
      @save="save"
      @cancel="store.closeKitEditor()"
      @archive="archive"
      @restore="restore"
      @reload="store.reloadKit()"
      @add-photos="store.addKitPhotos($event)"
      @cover-photo="coverPhoto"
      @remove-photo="store.removeKitPhoto($event)"
    />
  </div>
</template>

<style scoped>
.panel {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 16px;
  width: 100%;
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
.toolbar {
  display: flex;
  align-items: flex-end;
  gap: 12px;
  width: 100%;
}
.archived {
  display: flex;
  align-items: center;
  gap: 6px;
  height: var(--enc-control-height, 32px);
}
.new {
  margin-left: auto;
}
</style>
