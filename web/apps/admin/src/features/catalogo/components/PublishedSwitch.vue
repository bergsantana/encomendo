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

/* Phone: the box is drawn smaller inside a 48 px square, so the touch target is thumb-sized. */
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
    top: 14px;
    left: 20px;
    width: 8px;
    height: 14px;
    border: solid var(--enc-primary-text);
    border-width: 0 3px 3px 0;
    transform: rotate(45deg);
  }
  .switch:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
}
</style>
