<script setup lang="ts">
/**
 * NoticesPanel is "Avisar o fornecedor", above the tabs of Compras: sent
 * lines that are no longer needed, with the link that asks the supplier to
 * cancel ("Avisar pelo WhatsApp") and "Marcar como resolvido". The link is the
 * API's and never names the client.
 */
import type { Notice } from '@encomendo/api'
import { BaseButton, IconAlertTriangle } from '@encomendo/ui'

import { noticeReasonText, noticesHeading } from '../labels'

defineProps<{
  notices: readonly Notice[]
  /** busyId is the line being resolved; its buttons wait. */
  busyId: string | null
  /** status is "Resolvido. A linha foi cancelada." after a resolve. */
  status: string | null
  error: string | null
}>()

const emit = defineEmits<{
  notify: [notice: Notice]
  resolve: [lineId: string]
}>()
</script>

<template>
  <section
    v-if="notices.length > 0 || status || error"
    class="notices"
    aria-label="Avisar o fornecedor"
  >
    <p
      v-if="status"
      role="status"
      class="strong"
    >
      {{ status }}
    </p>
    <p
      v-if="error"
      role="alert"
      class="strong error"
    >
      {{ error }}
    </p>
    <template v-if="notices.length > 0">
      <h2>
        <IconAlertTriangle
          :size="16"
          stroke-width="2"
          aria-hidden="true"
        />
        {{ noticesHeading(notices.length) }}
      </h2>
      <ul>
        <li
          v-for="notice in notices"
          :key="notice.lineId"
        >
          <p class="strong">
            <span class="enc-code">{{ notice.number }}</span> · {{ notice.supplierName }}
          </p>
          <p class="enc-number">
            {{ notice.quantity }}× {{ notice.name }}
          </p>
          <p class="muted">
            {{ noticeReasonText(notice.reason, notice.orderNumber, notice.name) }}
          </p>
          <div class="actions">
            <BaseButton
              :disabled="busyId === notice.lineId"
              @click="emit('notify', notice)"
            >
              Avisar pelo WhatsApp
            </BaseButton>
            <BaseButton
              :disabled="busyId === notice.lineId"
              @click="emit('resolve', notice.lineId)"
            >
              Marcar como resolvido
            </BaseButton>
          </div>
        </li>
      </ul>
    </template>
  </section>
</template>

<style scoped>
.notices {
  display: flex;
  flex-direction: column;
  gap: 8px;
  width: 100%;
  box-sizing: border-box;
  padding: 12px 16px;
  border: 1px solid var(--enc-status-alert-text);
  border-radius: var(--enc-radius-card);
  background: var(--enc-status-alert-fill);
}
h2,
p {
  margin: 0;
}
h2 {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 15px;
  color: var(--enc-status-alert-text);
}
ul {
  display: flex;
  flex-direction: column;
  gap: 12px;
  margin: 0;
  padding: 0;
  list-style: none;
}
li {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 12px;
  border-radius: var(--enc-radius-card);
  background: var(--enc-surface);
}
.strong {
  font-weight: 600;
}
.error {
  color: var(--enc-status-alert-text);
}
.muted {
  color: var(--enc-ink-secondary);
}
.actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

/* Phone: each button fills the width, one under the other. */
@media (max-width: 640px) {
  .actions > .enc-button {
    flex: 1 1 100%;
  }
}
</style>
