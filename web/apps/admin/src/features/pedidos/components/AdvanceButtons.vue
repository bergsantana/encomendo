<script setup lang="ts">
/**
 * AdvanceButtons is "Próximo status": one "Mover para {status}" per status
 * the API says the order can move to (never one the client worked out), and,
 * under them, the message of the guard that refused the last move. The
 * status words are the order's own workflow's.
 */
import type { Order } from '@encomendo/api'
import { BaseButton } from '@encomendo/ui'

defineProps<{
  moves: Order['moves']
  guardMessage: string | null
  busy: boolean
}>()

const emit = defineEmits<{
  advance: [status: Order['moves'][number]]
}>()
</script>

<template>
  <section
    v-if="moves.length > 0"
    class="advance"
    aria-labelledby="advance-heading"
  >
    <h2 id="advance-heading">
      Próximo status
    </h2>
    <div class="buttons">
      <BaseButton
        v-for="move in moves"
        :key="move.key"
        :disabled="busy"
        @click="emit('advance', move)"
      >
        Mover para {{ move.label }}
      </BaseButton>
    </div>
    <p
      v-if="guardMessage"
      role="alert"
    >
      {{ guardMessage }}
    </p>
  </section>
</template>

<style scoped>
.advance {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
h2,
p {
  margin: 0;
}
h2 {
  font-size: 15px;
}
.buttons {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
[role='alert'] {
  color: var(--enc-status-alert-text);
  font-weight: 600;
}

@media (max-width: 640px) {
  .buttons {
    flex-direction: column;
  }
}
</style>
