<script setup lang="ts">
/**
 * BaseDialog is the one modal surface: a centered dialog, or a drawer from
 * the right for forms. It names itself by its title, moves the keyboard in
 * when it opens, keeps Tab inside, closes on Escape, and hands focus back to
 * what opened it. Closing is the parent's decision: it only asks (`close`).
 *
 * On phones (640 px and narrower, see responsive.ts) both variants become a
 * full-screen sheet: no backdrop to tap, and no Escape key on a phone, so
 * the slot must always carry its own "Fechar" or "Cancelar" button.
 */
import { nextTick, onBeforeUnmount, onMounted, useId, useTemplateRef } from 'vue'

import { trapTab } from '../focusTrap'

withDefaults(
  defineProps<{
    /** pt-BR title; it is also the dialog's accessible name. */
    title: string
    variant?: 'dialog' | 'drawer'
  }>(),
  { variant: 'dialog' },
)

const emit = defineEmits<{ close: [] }>()

const titleId = useId()
const surface = useTemplateRef<HTMLElement>('surface')
let opener: Element | null = null

onMounted(async () => {
  opener = document.activeElement
  await nextTick()
  // The surface itself, so a screen reader reads the title before the first field.
  surface.value?.focus()
})

onBeforeUnmount(() => {
  if (opener instanceof HTMLElement && opener.isConnected) opener.focus()
})

function onKeydown(event: KeyboardEvent): void {
  if (event.key === 'Escape') {
    event.stopPropagation()
    emit('close')
    return
  }
  if (surface.value !== null) trapTab(event, surface.value)
}
</script>

<template>
  <div
    class="enc-dialog"
    :class="`enc-dialog--${variant}`"
  >
    <div
      class="enc-dialog__backdrop"
      @click="emit('close')"
    />
    <div
      ref="surface"
      class="enc-dialog__surface"
      role="dialog"
      aria-modal="true"
      :aria-labelledby="titleId"
      tabindex="-1"
      @keydown="onKeydown"
    >
      <header class="enc-dialog__header">
        <h2
          :id="titleId"
          class="enc-dialog__title"
        >
          {{ title }}
        </h2>
      </header>
      <div class="enc-dialog__body">
        <slot />
      </div>
    </div>
  </div>
</template>

<style scoped>
.enc-dialog {
  position: fixed;
  inset: 0;
  z-index: 10;
  display: flex;
  justify-content: center;
  align-items: center;
}
.enc-dialog--drawer {
  justify-content: flex-end;
  align-items: stretch;
}
.enc-dialog__backdrop {
  position: absolute;
  inset: 0;
  background: rgb(27 29 32 / 40%);
}
.enc-dialog__surface {
  position: relative;
  display: flex;
  flex-direction: column;
  max-height: 100%;
  background: var(--enc-surface);
  border-radius: var(--enc-radius-card);
  box-shadow: 0 4px 24px rgb(27 29 32 / 20%);
}
.enc-dialog--dialog .enc-dialog__surface {
  width: min(640px, calc(100% - 32px));
}
.enc-dialog--drawer .enc-dialog__surface {
  width: min(560px, 100%);
  border-radius: 0;
}
.enc-dialog__surface:focus {
  outline: none;
}
.enc-dialog__header {
  padding: 16px 24px;
  border-bottom: 1px solid var(--enc-hairline);
}
.enc-dialog__title {
  margin: 0;
  font-size: 18px;
}
.enc-dialog__body {
  overflow-y: auto;
  padding: 16px 24px 24px;
}

/* Phone: a full-screen sheet. After the variants, with the same specificity, so it wins. */
@media (max-width: 640px) {
  .enc-dialog .enc-dialog__surface {
    width: 100%;
    height: 100%;
    max-height: 100%;
    border-radius: 0;
    box-shadow: none;
  }
  .enc-dialog__header {
    padding: 12px 16px;
  }
  .enc-dialog__body {
    flex: 1;
    padding: 16px;
    /*
     * Keeps a focused field clear of the sticky `enc-sheet-actions` footer
     * (tokens.css), even when its buttons wrap to two rows (about 130 px).
     */
    scroll-padding-bottom: 144px;
  }
}
</style>
