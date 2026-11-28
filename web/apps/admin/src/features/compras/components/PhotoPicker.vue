<script setup lang="ts">
/**
 * PhotoPicker is "Foto do item" or "Foto da nota fiscal": a file input that
 * takes any image (`accept="image/*"`), so a phone, and the Android shell,
 * offer the camera or the gallery, then the photos chosen with "Remover
 * foto" on each. While one goes up it says "Enviando a foto…". It holds no
 * state and sends nothing: the screen's store uploads, limits and keeps them.
 */
import { BaseButton, IconCamera } from '@encomendo/ui'
import { computed, useId } from 'vue'

import type { ReceiptPhoto } from '../receipt'

const props = withDefaults(defineProps<{
  /** label is the words of the button: "Foto do item" or "Foto da nota fiscal". */
  label: string
  photos: readonly ReceiptPhoto[]
  /** subject is what the photos are of, so a "Remover foto" button names its photo for a screen reader. */
  subject: string
  error: string | null
  /** named adds the subject to the button's accessible name ("Foto do item: Banco"), so several pickers on a screen can be told apart. The visible words stay. */
  named?: boolean
  /** disabled holds the picker still, while a receipt is being saved. */
  disabled?: boolean
}>(), { named: false, disabled: false })

const emit = defineEmits<{
  add: [files: File[]]
  remove: [photoId: number]
}>()

const inputId = useId()
const uploading = computed(() => props.photos.some((photo) => photo.key === null))

function chosen(event: Event): void {
  const input = event.target
  if (!(input instanceof HTMLInputElement) || input.files === null) return
  const files = [...input.files]
  // The same photo can be chosen again after a refusal.
  input.value = ''
  if (files.length > 0) emit('add', files)
}
</script>

<template>
  <div class="picker">
    <input
      :id="inputId"
      class="enc-visually-hidden"
      type="file"
      multiple
      accept="image/*"
      :disabled="disabled"
      @change="chosen"
    >
    <label
      class="add"
      :for="inputId"
    >
      <IconCamera
        :size="24"
        aria-hidden="true"
      />
      {{ label }}<span
        v-if="named"
        class="enc-visually-hidden"
      >: {{ subject }}</span>
    </label>
    <ul
      v-if="photos.length > 0"
      class="photos"
    >
      <li
        v-for="(photo, index) in photos"
        :key="photo.id"
      >
        <span>Foto {{ index + 1 }}</span>
        <BaseButton
          :disabled="disabled"
          :aria-label="`Remover foto ${index + 1}: ${subject}`"
          @click="emit('remove', photo.id)"
        >
          Remover foto
        </BaseButton>
      </li>
    </ul>
    <p
      v-if="uploading"
      role="status"
      class="hint"
    >
      Enviando a foto…
    </p>
    <p
      v-if="error"
      role="alert"
      class="error"
    >
      {{ error }}
    </p>
  </div>
</template>

<style scoped>
.picker {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 8px;
}
.add {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  box-sizing: border-box;
  min-height: var(--enc-control-height, 32px);
  padding: 0 14px;
  border: 1px solid var(--enc-secondary-border);
  border-radius: var(--enc-radius-card);
  background: var(--enc-surface);
  color: var(--enc-ink);
  font-weight: 600;
  cursor: pointer;
}
input:disabled + .add {
  cursor: not-allowed;
  opacity: 0.5;
}
input:focus-visible + .add {
  outline: 2px solid var(--enc-accent);
  outline-offset: 2px;
}
.photos {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin: 0;
  padding: 0;
  list-style: none;
}
.photos li {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 12px;
}
p {
  margin: 0;
}
.hint {
  color: var(--enc-ink-secondary);
}
.error {
  color: var(--enc-status-alert-text);
  font-weight: 600;
}
</style>
