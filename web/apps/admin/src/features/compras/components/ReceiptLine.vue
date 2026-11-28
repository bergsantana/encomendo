<script setup lang="ts">
/**
 * ReceiptLine is one line of Receber itens: "{qtd}× {item}", its choices and
 * "Pedido P-0231", with two large buttons, "Tudo certo" and "Com defeito".
 * A checked line offers "Foto do item" (up to 2); a defective one also asks
 * "O que houve?", which "Salvar recebimento" requires. Pressing the same
 * button again takes the check back. It sends nothing: the screen's store
 * keeps the checks, the notes and the photos.
 */
import type { ToReceiveLine } from '@encomendo/api'
import { BaseButton, IconAlertTriangle, IconCheck } from '@encomendo/ui'
import { useId } from 'vue'

import { choicesText } from '../labels'
import type { EntryMark } from '../receipt'
import PhotoPicker from './PhotoPicker.vue'

defineProps<{
  line: ToReceiveLine
  /** mark is what was said about this line, or null while it is unchecked. */
  mark: EntryMark | null
  /** noteError asks for the defect's note ("Conte o que houve com {item}."), or is null. */
  noteError: string | null
  photoError: string | null
  /** locked holds the line still, while a receipt is being saved: what is typed or checked now would not be in it. */
  locked: boolean
}>()

const emit = defineEmits<{
  check: [check: 'ok' | 'defective']
  note: [text: string]
  addPhotos: [files: File[]]
  removePhoto: [photoId: number]
}>()

const noteId = useId()
</script>

<template>
  <li class="line">
    <p class="what enc-number">
      {{ line.quantity }}× {{ line.name }}
    </p>
    <p
      v-if="line.choices.length > 0"
      class="muted"
    >
      {{ choicesText(line.choices) }}
    </p>
    <p class="muted">
      Pedido <span class="enc-code">{{ line.orderNumber }}</span>
    </p>

    <div class="checks">
      <BaseButton
        class="check"
        :class="{ chosen: mark?.check === 'ok' }"
        :disabled="locked"
        :aria-pressed="mark?.check === 'ok'"
        :aria-label="`Tudo certo: ${line.name}`"
        @click="emit('check', 'ok')"
      >
        <IconCheck
          v-if="mark?.check === 'ok'"
          :size="24"
          aria-hidden="true"
        />
        Tudo certo
      </BaseButton>
      <BaseButton
        class="check"
        :class="{ chosen: mark?.check === 'defective' }"
        :disabled="locked"
        :aria-pressed="mark?.check === 'defective'"
        :aria-label="`Com defeito: ${line.name}`"
        @click="emit('check', 'defective')"
      >
        <IconAlertTriangle
          v-if="mark?.check === 'defective'"
          :size="24"
          aria-hidden="true"
        />
        Com defeito
      </BaseButton>
    </div>

    <template v-if="mark">
      <div
        v-if="mark.check === 'defective'"
        class="note"
      >
        <label :for="noteId">O que houve?</label>
        <textarea
          :id="noteId"
          rows="3"
          maxlength="500"
          :value="mark.note"
          :disabled="locked"
          :aria-invalid="noteError ? 'true' : undefined"
          :aria-describedby="noteError ? `${noteId}-error` : undefined"
          @input="emit('note', ($event.target as HTMLTextAreaElement).value)"
        />
        <p
          v-if="noteError"
          :id="`${noteId}-error`"
          role="alert"
          class="error"
        >
          {{ noteError }}
        </p>
      </div>
      <PhotoPicker
        label="Foto do item"
        :photos="mark.photos"
        :subject="line.name"
        :error="photoError"
        named
        :disabled="locked"
        @add="emit('addPhotos', $event)"
        @remove="emit('removePhoto', $event)"
      />
    </template>
  </li>
</template>

<style scoped>
.line {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 8px;
  padding: 12px;
  border: 1px solid var(--enc-hairline);
  border-radius: var(--enc-radius-card);
  background: var(--enc-surface);
}
p {
  margin: 0;
}
.muted {
  color: var(--enc-ink-secondary);
}
.checks {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-self: stretch;
}
.check {
  display: inline-flex;
  flex: 1 1 140px;
  align-items: center;
  justify-content: center;
  gap: 8px;
}
/* Selection is the one use of the accent besides focus: a ring, plus the icon and the heavier border, never color alone. */
.check.chosen {
  border: 2px solid var(--enc-ink);
  box-shadow: 0 0 0 3px var(--enc-accent);
}
.note {
  display: flex;
  flex-direction: column;
  gap: 4px;
  align-self: stretch;
}
.note label {
  font-weight: 600;
}
textarea {
  padding: 8px 10px;
  border: 1px solid var(--enc-secondary-border);
  border-radius: var(--enc-radius-card);
  background: var(--enc-surface);
  color: var(--enc-ink);
  font: inherit;
}
textarea[aria-invalid='true'] {
  border-color: var(--enc-status-alert-text);
}
.error {
  color: var(--enc-status-alert-text);
  font-weight: 600;
}
</style>
