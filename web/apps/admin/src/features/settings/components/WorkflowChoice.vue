<script setup lang="ts">
/**
 * WorkflowChoice is the "Lista de status" section: one card per preset with
 * its steps, the one in use marked "Em uso" and a "Usar esta lista" button on
 * the others. The hint says the choice only applies to new orders.
 */
import type { StatusWorkflow } from '@encomendo/api'
import { BaseButton } from '@encomendo/ui'
import { useId } from 'vue'

import WorkflowPreview from './WorkflowPreview.vue'

defineProps<{
  workflows: readonly StatusWorkflow[]
  /** currentId and currentVersion name the preset the store uses now. */
  currentId: string
  currentVersion: number
  /** choosingId is the preset being saved; every button waits while one is. */
  choosingId: string | null
  notice: string | null
  error: string | null
  /** conflict offers "Recarregar" beside the error. */
  conflict: boolean
}>()

const emit = defineEmits<{
  choose: [workflow: StatusWorkflow]
  reload: []
}>()

const prefix = useId()
</script>

<template>
  <section
    class="section"
    aria-labelledby="lista-de-status"
  >
    <h2 id="lista-de-status">
      Lista de status
    </h2>
    <p class="hint">
      Os status que você e seus clientes veem em cada pedido. Vale para os pedidos novos; os pedidos que já existem continuam com a lista em que foram criados.
    </p>
    <p
      v-if="notice"
      role="status"
      class="notice"
    >
      {{ notice }}
    </p>
    <div
      v-if="error"
      class="alert"
      role="alert"
    >
      <p>{{ error }}</p>
      <BaseButton
        v-if="conflict"
        @click="emit('reload')"
      >
        Recarregar
      </BaseButton>
    </div>
    <div class="presets">
      <article
        v-for="workflow in workflows"
        :key="workflow.id"
        class="preset"
        :class="{ 'preset--current': workflow.id === currentId && workflow.version === currentVersion }"
        :aria-labelledby="`${prefix}-${workflow.id}`"
      >
        <header class="preset__header">
          <h3 :id="`${prefix}-${workflow.id}`">
            {{ workflow.name }}
          </h3>
          <span
            v-if="workflow.id === currentId && workflow.version === currentVersion"
            class="current"
          >Em uso</span>
          <BaseButton
            v-else
            :disabled="choosingId !== null"
            :aria-describedby="`${prefix}-${workflow.id}`"
            @click="emit('choose', workflow)"
          >
            Usar esta lista
          </BaseButton>
        </header>
        <WorkflowPreview :statuses="workflow.statuses" />
      </article>
    </div>
  </section>
</template>

<style scoped>
.section {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
h2,
h3,
p {
  margin: 0;
}
h2 {
  font-size: 18px;
}
h3 {
  font-size: 15px;
}
.hint {
  color: var(--enc-ink-secondary);
}
.notice,
.alert p {
  font-weight: 600;
}
.alert {
  display: flex;
  align-items: center;
  gap: 12px;
  color: var(--enc-status-alert-text);
}
.presets {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 16px;
}
.preset {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 16px;
  border: 1px solid var(--enc-hairline);
  border-radius: var(--enc-radius-card);
  background: var(--enc-surface);
}
.preset--current {
  border-color: var(--enc-secondary-border);
  box-shadow: inset 3px 0 0 var(--enc-accent);
}
.preset__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}
.current {
  font-weight: 600;
}

/* Phone: one preset per row, its button as wide as the card. */
@media (max-width: 640px) {
  .presets {
    grid-template-columns: minmax(0, 1fr);
  }
  .preset__header {
    flex-wrap: wrap;
  }
  .preset__header > .enc-button {
    flex: 1 1 100%;
  }
  .alert {
    flex-wrap: wrap;
  }
}
</style>
