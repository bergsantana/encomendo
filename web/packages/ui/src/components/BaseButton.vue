<script setup lang="ts">
/**
 * BaseButton is the only button. Use variant "primary" once per screen
 * (graphite); everything else is "secondary".
 */
withDefaults(
  defineProps<{
    variant?: 'primary' | 'secondary'
    type?: 'button' | 'submit'
    disabled?: boolean
  }>(),
  { variant: 'secondary', type: 'button', disabled: false },
)

defineEmits<{ click: [event: MouseEvent] }>()
</script>

<template>
  <button
    class="enc-button"
    :class="`enc-button--${variant}`"
    :type="type"
    :disabled="disabled"
    @click="$emit('click', $event)"
  >
    <slot />
  </button>
</template>

<style scoped>
.enc-button {
  height: var(--enc-control-height, 32px);
  padding: 0 14px;
  border-radius: var(--enc-radius-card);
  font: inherit;
  font-weight: 600;
  cursor: pointer;
}
.enc-button:disabled {
  cursor: not-allowed;
  opacity: 0.5;
}
.enc-button--primary {
  background: var(--enc-primary);
  color: var(--enc-primary-text);
  border: 1px solid var(--enc-primary);
}
.enc-button--secondary {
  background: var(--enc-surface);
  color: var(--enc-ink);
  border: 1px solid var(--enc-secondary-border);
}
</style>
