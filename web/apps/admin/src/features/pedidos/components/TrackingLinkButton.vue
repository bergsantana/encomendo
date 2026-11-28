<script setup lang="ts">
/**
 * TrackingLinkButton is "Enviar link de acompanhamento": it asks for a new
 * link and opens WhatsApp with it. The hint says the old link stops working.
 */
import { BaseButton } from '@encomendo/ui'

defineProps<{
  busy: boolean
}>()

const emit = defineEmits<{
  send: []
}>()
</script>

<template>
  <div class="tracking">
    <BaseButton
      :disabled="busy"
      @click="emit('send')"
    >
      Enviar link de acompanhamento
    </BaseButton>
    <p class="hint">
      Cada envio cria um link novo. O link anterior deixa de funcionar.
    </p>
  </div>
</template>

<style scoped>
.tracking {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.hint {
  margin: 0;
  color: var(--enc-ink-secondary);
}
</style>
