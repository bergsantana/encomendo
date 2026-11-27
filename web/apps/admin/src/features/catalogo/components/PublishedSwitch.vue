<script setup lang="ts">
/**
 * PublishedSwitch is the "No portal" switch: whether an item shows on the
 * store page. It is controlled: it shows what the server saved, so a refused
 * change leaves it as it was.
 */
defineProps<{
  checked: boolean
  /** label is the accessible name, "Mostrar {item} na página da loja". Leave it out when a visible <label for> names the switch. */
  label?: string
  /** id lets a visible <label for> name the switch. */
  id?: string
  disabled?: boolean
  /** busy waits while the change is saved: the switch is off limits until the server answers. */
  busy?: boolean
}>()

const emit = defineEmits<{ toggle: [published: boolean] }>()

function onChange(event: Event): void {
  const input = event.target
  if (!(input instanceof HTMLInputElement)) return
  const next = input.checked
  // Put the box back; it moves only when the saved item says so.
  input.checked = !next
  emit('toggle', next)
}
</script>

<template>
  <input
    :id="id"
    class="switch"
    type="checkbox"
    role="switch"
    :checked="checked"
    :disabled="disabled || busy"
    :aria-busy="busy ? 'true' : undefined"
    :aria-label="label"
    @change="onChange"
  >
</template>

<style scoped>
.switch {
  width: 18px;
  height: 18px;
  accent-color: var(--enc-primary);
}

/*
 * Phone: a box drawn inside a square as tall as a control (48 px), so the touch
 * target is thumb-sized. The box is the square minus 10 px on each side (28 px at
 * 48), and the tick sits at the square's centre.
 */
@media (max-width: 640px) {
  .switch {
    appearance: none;
    position: relative;
    display: block;
    flex: none;
    width: var(--enc-control-height);
    height: var(--enc-control-height);
    margin: 0;
    background: none;
    cursor: pointer;
  }
  .switch::before {
    content: '';
    position: absolute;
    inset: 10px;
    border: 2px solid var(--enc-secondary-border);
    border-radius: 4px;
    background: var(--enc-surface);
  }
  .switch:checked::before {
    border-color: var(--enc-primary);
    background: var(--enc-primary);
  }
  .switch:checked::after {
    content: '';
    position: absolute;
    top: 44%;
    left: 50%;
    width: 8px;
    height: 14px;
    border: solid var(--enc-primary-text);
    border-width: 0 3px 3px 0;
    transform: translate(-50%, -50%) rotate(45deg);
  }
  .switch:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
}

/* Forced colors replace the custom colours: keep the box and the tick apart in the system's own. */
@media (max-width: 640px) and (forced-colors: active) {
  .switch::before {
    border-color: ButtonText;
    background: Canvas;
  }
  .switch:checked::before {
    border-color: Highlight;
    background: Highlight;
  }
  .switch:checked::after {
    border-color: HighlightText;
  }
}
</style>
