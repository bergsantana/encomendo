<script setup lang="ts">
/**
 * KitDrawer is "Novo kit" / "Editar kit": the whole kit form. It holds the
 * draft and hands it to the store on "Salvar". After a save it shows the kit
 * the API returned, so the totals and days are the new ones. Roles without
 * `catalog:write` see the same fields, disabled, with no buttons that change
 * anything. Margem shows only when the API sent it (ADR 0025).
 */
import type { Attribute, Category, ItemSummary, Kit, Photo } from '@encomendo/api'
import { BaseButton, BaseDialog, BaseInput } from '@encomendo/ui'
import { computed, nextTick, reactive, ref, useId, useTemplateRef, watch } from 'vue'

import { formatDays } from '../../../format'
import { niche } from '../../../niche'
import { draftFromKit, emptyKitDraft, type KitDraft, type SwapDiscount } from '../kitForm'
import AttributePicker from './AttributePicker.vue'
import DiscardQuestion from './DiscardQuestion.vue'
import KitItemsEditor from './KitItemsEditor.vue'
import KitTotals from './KitTotals.vue'
import PhotosEditor from './PhotosEditor.vue'
import PublishedSwitch from './PublishedSwitch.vue'

const props = defineProps<{
  /** kit is the kit being edited as last saved, or null for a new one. */
  kit: Kit | null
  canWrite: boolean
  catalogItems: readonly ItemSummary[]
  categories: readonly Category[]
  attributes: readonly Attribute[]
  /** defaultAssemblyDays and defaultSafetyMarginDays are the store's, for the "Padrão da loja" hints. */
  defaultAssemblyDays: number | null
  defaultSafetyMarginDays: number | null
  saving: boolean
  /** formError is the message above the form; `fieldErrors` the ones next to the fields. */
  formError: string | null
  fieldErrors: Readonly<Record<string, string>>
  /** conflict offers "Recarregar" beside the message. */
  conflict: boolean
  /** notice is "Kit salvo." after a save. */
  notice: string | null
  photos: readonly Photo[]
  photoError: string | null
  photoBusy: boolean
}>()

const emit = defineEmits<{
  save: [draft: KitDraft]
  cancel: []
  archive: []
  restore: []
  reload: []
  addPhotos: [files: File[]]
  coverPhoto: [photo: Photo]
  removePhoto: [photo: Photo]
}>()

const draft = reactive<KitDraft>(props.kit === null ? emptyKitDraft() : draftFromKit(props.kit))
/** initial is the form as it opened or was last saved, to tell whether anything was typed since. */
const initial = ref(JSON.stringify(draft))
const discarding = ref(false)
const descriptionId = useId()
const publishedId = useId()
const swapName = useId()
const status = useTemplateRef<HTMLElement>('status')

const archived = computed(() => props.kit?.archived === true)
const edited = computed(() => JSON.stringify(draft) !== initial.value)
const title = computed(() => (!props.canWrite ? 'Kit' : props.kit === null ? 'Novo kit' : 'Editar kit'))
const swapModes: { value: SwapDiscount; label: string; hint: string }[] = [
  { value: 'keep', label: 'Manter o desconto do kit', hint: 'Preço do kit − item retirado + item novo, pelos preços avulsos.' },
  { value: 'lose', label: 'Cobrar a soma dos itens avulsos', hint: 'O kit trocado custa a soma dos preços avulsos dos seus itens.' },
]

function defaultHint(days: number | null): string | null {
  return days === null ? null : `Padrão da loja: ${formatDays(days)}`
}

// A kit just saved, or read again, replaces what the form holds.
watch(
  () => props.kit,
  (kit) => {
    Object.assign(draft, kit === null ? emptyKitDraft() : draftFromKit(kit))
    initial.value = JSON.stringify(draft)
    discarding.value = false
  },
)

// The keyboard follows "Kit salvo.", so it is read out.
watch(
  () => props.notice,
  async (notice) => {
    if (notice === null) return
    await nextTick()
    status.value?.focus()
  },
)

/**
 * requestClose is every way out that drops the draft (Cancelar, Escape, a
 * click outside). With unsaved edits it asks first, so a stray click cannot
 * throw them away.
 */
function requestClose(): void {
  if (!props.canWrite || !edited.value) {
    emit('cancel')
    return
  }
  discarding.value = true
}
</script>

<template>
  <BaseDialog
    :title="title"
    variant="drawer"
    @close="requestClose"
  >
    <form
      class="form"
      novalidate
      @submit.prevent="emit('save', draft)"
    >
      <p
        v-if="notice"
        ref="status"
        role="status"
        tabindex="-1"
        class="notice"
      >
        {{ notice }}
      </p>
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
          <label :for="descriptionId">Descrição</label>
          <textarea
            :id="descriptionId"
            v-model="draft.description"
            rows="3"
          />
        </div>
        <BaseInput
          v-model="draft.price"
          label="Preço do kit"
          inputmode="decimal"
          :error="fieldErrors.price ?? null"
        />

        <KitItemsEditor
          v-model="draft.items"
          :catalog-items="catalogItems"
          :categories="categories"
          :can-write="canWrite"
          :errors="fieldErrors"
        />

        <fieldset class="swap">
          <legend>Ao trocar um item</legend>
          <div
            v-for="mode in swapModes"
            :key="mode.value"
            class="mode"
          >
            <label class="check">
              <input
                v-model="draft.swapDiscount"
                type="radio"
                :name="swapName"
                :value="mode.value"
                :aria-describedby="`${swapName}-${mode.value}`"
              >
              {{ mode.label }}
            </label>
            <p
              :id="`${swapName}-${mode.value}`"
              class="hint"
            >
              {{ mode.hint }}
            </p>
          </div>
        </fieldset>

        <BaseInput
          v-model="draft.assemblyDays"
          :label="`Dias de ${niche.montagem}`"
          inputmode="numeric"
          :hint="defaultHint(defaultAssemblyDays)"
          :error="fieldErrors.assemblyDays ?? null"
        />
        <BaseInput
          v-model="draft.safetyMarginDays"
          label="Margem de segurança (dias)"
          inputmode="numeric"
          :hint="defaultHint(defaultSafetyMarginDays)"
          :error="fieldErrors.safetyMarginDays ?? null"
        />

        <KitTotals
          :kit="kit"
          :edited="edited"
        />

        <AttributePicker
          v-model="draft.attributeValueIds"
          :attributes="attributes"
        />
      </fieldset>

      <PhotosEditor
        :photos="photos"
        :saved="kit !== null"
        subject="kit"
        :can-write="canWrite"
        :busy="photoBusy"
        :error="photoError"
        @add="(files) => emit('addPhotos', files)"
        @cover="(photo) => emit('coverPhoto', photo)"
        @remove="(photo) => emit('removePhoto', photo)"
      />

      <div class="check">
        <PublishedSwitch
          :id="publishedId"
          :checked="draft.published && !archived"
          :disabled="!canWrite || archived"
          @toggle="(published) => (draft.published = published)"
        />
        <label :for="publishedId">Mostrar na página da loja</label>
      </div>

      <DiscardQuestion
        v-if="discarding"
        @keep="discarding = false"
        @discard="emit('cancel')"
      />

      <div class="buttons">
        <template v-if="canWrite">
          <BaseButton
            variant="primary"
            type="submit"
            :disabled="saving"
          >
            Salvar
          </BaseButton>
          <BaseButton @click="requestClose">
            Cancelar
          </BaseButton>
          <BaseButton
            v-if="kit !== null && !archived"
            @click="emit('archive')"
          >
            Arquivar
          </BaseButton>
          <BaseButton
            v-if="kit !== null && archived"
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
.notice {
  margin: 0;
  font-weight: 600;
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
.swap {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin: 0;
  padding: 12px;
  border: 1px solid var(--enc-hairline);
  border-radius: var(--enc-radius-card);
}
legend {
  padding: 0 4px;
  font-weight: 600;
}
.mode {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.hint {
  margin: 0 0 0 24px;
  color: var(--enc-ink-secondary);
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
