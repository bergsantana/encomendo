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
</style>
