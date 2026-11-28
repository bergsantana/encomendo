<script setup lang="ts">
/**
 * OrderHistory is "Histórico": what happened to the order, newest first, as
 * "{quem} {o quê}" with its date and time ("Ana moveu para Em montagem").
 */
import type { HistoryEntry, Order } from '@encomendo/api'
import { BaseButton } from '@encomendo/ui'

import { historyText } from '../historyText'

defineProps<{
  entries: readonly HistoryEntry[]
  items: Order['items']
  timeZone?: string
  loading: boolean
  error: string | null
  hasMore: boolean
}>()

const emit = defineEmits<{
  more: []
}>()

function when(instant: string, timeZone: string | undefined): string {
  return new Intl.DateTimeFormat('pt-BR', { dateStyle: 'short', timeStyle: 'short', timeZone }).format(new Date(instant))
}
</script>

<template>
  <section
    class="history"
    aria-labelledby="history-heading"
  >
    <h2 id="history-heading">
      Histórico
    </h2>
    <p
      v-if="error"
      role="alert"
    >
      {{ error }}
    </p>
    <ol v-if="entries.length > 0">
      <li
        v-for="(entry, index) in entries"
        :key="`${entry.at}-${index}`"
      >
        <span>{{ historyText(entry, items) }}</span>
        <time
          class="muted"
          :datetime="entry.at"
        >{{ when(entry.at, timeZone) }}</time>
      </li>
    </ol>
    <p
      v-else-if="loading"
      class="muted"
    >
      Carregando o histórico…
    </p>
    <BaseButton
      v-if="hasMore"
      :disabled="loading"
      @click="emit('more')"
    >
      Carregar mais
    </BaseButton>
  </section>
</template>

<style scoped>
.history {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 8px;
}
h2,
p {
  margin: 0;
}
h2 {
  font-size: 15px;
}
ol {
  display: flex;
  flex-direction: column;
  width: 100%;
  margin: 0;
  padding: 0;
  list-style: none;
}
li {
  display: flex;
  flex-wrap: wrap;
  justify-content: space-between;
  gap: 2px 16px;
  padding: 6px 0;
  border-bottom: 1px solid var(--enc-hairline);
}
.muted {
  color: var(--enc-ink-secondary);
}
[role='alert'] {
  color: var(--enc-status-alert-text);
  font-weight: 600;
}

@media (max-width: 640px) {
  .history {
    align-items: stretch;
  }
  li {
    flex-direction: column;
  }
}
</style>
