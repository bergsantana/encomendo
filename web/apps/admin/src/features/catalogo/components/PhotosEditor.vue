<script setup lang="ts">
/**
 * PhotosEditor is "Fotos": the photos of a saved item, the first being the
 * cover, with upload, "Usar como capa" and "Remover foto". Photos change on
 * the server at once, apart from "Salvar". A new item has no id to attach
 * photos to yet, so it asks to be saved first.
 */
import type { Photo } from '@encomendo/api'
import { BaseButton } from '@encomendo/ui'
import { useId } from 'vue'

import { photoOwnerWord } from '../photoMessage'

withDefaults(
  defineProps<{
    photos: readonly Photo[]
    /** saved is false for a new item or kit. */
    saved: boolean
    /** subject is what the photos belong to, for the hint that asks a new one to be saved first */
    subject?: 'item' | 'kit'
    canWrite: boolean
    busy: boolean
    error: string | null
  }>(),
  { subject: 'item' },
)

const emit = defineEmits<{
  add: [files: File[]]
  cover: [photo: Photo]
  remove: [photo: Photo]
}>()

const inputId = useId()

function chosen(event: Event): void {
  const input = event.target
  if (!(input instanceof HTMLInputElement) || input.files === null) return
  const files = [...input.files]
  // The same file can be chosen again after a refusal.
  input.value = ''
  if (files.length > 0) emit('add', files)
}
</script>

<template>
  <fieldset class="photos">
    <legend>Fotos</legend>
    <p
      v-if="!saved"
      class="hint"
    >
      Salve o {{ photoOwnerWord[subject] }} para adicionar fotos.
    </p>
    <template v-else>
      <p
        v-if="error"
        role="alert"
        class="error"
      >
        {{ error }}
      </p>
      <ul v-if="photos.length > 0">
        <li
          v-for="(photo, index) in photos"
          :key="photo.id"
        >
          <img
            :src="photo.url"
            :alt="`Foto ${index + 1}`"
          >
          <span
            v-if="index === 0"
            class="cover"
          >Capa</span>
          <BaseButton
            v-else-if="canWrite"
            :aria-label="`Usar como capa, foto ${index + 1}`"
            @click="emit('cover', photo)"
          >
            Usar como capa
          </BaseButton>
          <BaseButton
            v-if="canWrite"
            :aria-label="`Remover foto ${index + 1}`"
            @click="emit('remove', photo)"
          >
            Remover foto
          </BaseButton>
        </li>
      </ul>
      <template v-if="canWrite">
        <label
          class="add"
          :for="inputId"
        >Adicionar fotos</label>
        <input
          :id="inputId"
          class="enc-visually-hidden"
          type="file"
          multiple
          accept="image/jpeg,image/png,image/webp"
          :disabled="busy"
          @change="chosen"
        >
        <p class="hint">
          JPEG, PNG ou WebP, até 10 MB. A primeira é a capa.
        </p>
        <p
          v-if="busy"
          role="status"
          class="hint"
        >
          Enviando a foto…
        </p>
      </template>
    </template>
  </fieldset>
</template>

<style scoped>
.photos {
  display: flex;
  flex-direction: column;
  gap: 12px;
  margin: 0;
  padding: 12px;
  border: 1px solid var(--enc-hairline);
  border-radius: var(--enc-radius-card);
}
legend {
  padding: 0 4px;
  font-weight: 600;
}
ul {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  margin: 0;
  padding: 0;
  list-style: none;
}
li {
  display: flex;
  flex-direction: column;
  gap: 6px;
  width: 128px;
}
img {
  width: 128px;
  height: 128px;
  object-fit: cover;
  border-radius: var(--enc-radius-card);
  background: var(--enc-ground);
}
.cover {
  font-weight: 600;
}
.add {
  display: inline-flex;
  align-items: center;
  align-self: flex-start;
  height: var(--enc-control-height, 32px);
  padding: 0 14px;
  border: 1px solid var(--enc-secondary-border);
  border-radius: var(--enc-radius-card);
  background: var(--enc-surface);
  font-weight: 600;
  cursor: pointer;
}
.add:has(+ input:focus-visible) {
  outline: 2px solid var(--enc-accent);
  outline-offset: 2px;
}
.hint {
  margin: 0;
  color: var(--enc-ink-secondary);
}
.error {
  margin: 0;
  color: var(--enc-status-alert-text);
  font-weight: 600;
}
</style>
