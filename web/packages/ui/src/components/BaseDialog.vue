<script setup lang="ts">
/**
 * BaseDialog is the one modal surface: a centered dialog, or a drawer from
 * the right for forms. It names itself by its title, moves the keyboard in
 * when it opens, keeps Tab inside, closes on Escape, and hands focus back to
 * what opened it. Closing is the parent's decision: it only asks (`close`).
 */
import { nextTick, onBeforeUnmount, onMounted, useId, useTemplateRef } from 'vue'

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

const focusable = 'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'

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
  if (event.key !== 'Tab' || surface.value === null) return
  const inside = [...surface.value.querySelectorAll<HTMLElement>(focusable)]
  const first = inside[0]
  const last = inside[inside.length - 1]
  if (first === undefined || last === undefined) return
  const active = document.activeElement
  if (event.shiftKey && (active === first || active === surface.value)) {
    event.preventDefault()
    last.focus()
  } else if (!event.shiftKey && active === last) {
    event.preventDefault()
    first.focus()
  }
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
</style>
