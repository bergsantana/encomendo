<script setup lang="ts">
/**
 * SupplierCard is one supplier of "A comprar": its name, its WhatsApp and
 * "{n} itens", then one line per pending item with a box (all checked at
 * first), "Ver mensagem" with the text the API built for the checked lines,
 * and "Enviar pelo WhatsApp". "Custo (por unidade)" shows only when the API
 * sent it, which is the owner's alone. Nothing here is worked out: the
 * message, the dates and the costs are the API's.
 */
import type { ToBuySupplier } from '@encomendo/api'
import { BaseButton, StatusBadge } from '@encomendo/ui'
import { computed, useId } from 'vue'
import { RouterLink } from 'vue-router'

import { formatCivilDate, formatMoney } from '../../../format'
import { formatWhatsapp } from '../../../phone'
import { choicesText, itemsCount } from '../labels'
import type { Preview } from '../store'

const props = defineProps<{
  supplier: ToBuySupplier
  /** unchecked holds the order items whose box is off. */
  unchecked: ReadonlySet<string>
  /** preview is this supplier's open "Ver mensagem", or null. */
  preview: Preview | null
  /** sending is true while a send is on its way; every card waits. */
  sending: boolean
}>()

const emit = defineEmits<{
  pick: [orderItemId: string, picked: boolean]
  togglePreview: []
  send: []
  copy: [text: string]
}>()

const headingId = useId()
const pickedCount = computed(() => props.supplier.items.filter((item) => !props.unchecked.has(item.orderItemId)).length)
</script>

<template>
  <article
    class="card"
    :aria-labelledby="headingId"
  >
    <header class="head">
      <h2 :id="headingId">
        {{ supplier.supplierName }}
      </h2>
      <p
        v-if="supplier.whatsappPhone"
        class="muted"
      >
        {{ formatWhatsapp(supplier.whatsappPhone) }}
      </p>
      <p class="muted">
        {{ itemsCount(supplier.items.length) }}
      </p>
    </header>

    <ul class="lines">
      <li
        v-for="item in supplier.items"
        :key="item.orderItemId"
        class="line"
      >
        <label class="enc-check">
          <input
            type="checkbox"
            :checked="!unchecked.has(item.orderItemId)"
            @change="emit('pick', item.orderItemId, ($event.target as HTMLInputElement).checked)"
          >
          <span class="enc-number">{{ item.quantity }}× {{ item.name }}</span>
        </label>
        <StatusBadge
          v-if="item.replacement"
          label="Reposição"
          phase="sourcing"
        />
        <p
          v-if="item.choices.length > 0"
          class="muted"
        >
          {{ choicesText(item.choices) }}
        </p>
        <p class="muted">
          Pedido
          <RouterLink
            class="enc-code"
            :to="{ name: 'order', params: { orderId: item.orderId } }"
          >
            {{ item.orderNumber }}
          </RouterLink>
        </p>
        <p class="muted">
          Precisa chegar até {{ formatCivilDate(item.arrivalLimit) }}
        </p>
        <p
          v-if="item.cost"
          class="muted"
        >
          Custo (por unidade) <span class="enc-number">{{ formatMoney(item.cost.cents, item.cost.currency) }}</span>
        </p>
      </li>
    </ul>

    <div class="actions">
      <BaseButton
        :disabled="pickedCount === 0"
        :aria-expanded="preview !== null"
        @click="emit('togglePreview')"
      >
        Ver mensagem
      </BaseButton>
      <BaseButton
        :disabled="pickedCount === 0 || sending"
        @click="emit('send')"
      >
        Enviar pelo WhatsApp
      </BaseButton>
    </div>
    <p
      class="hint"
      :role="pickedCount === 0 ? 'alert' : undefined"
    >
      {{ pickedCount === 0 ? 'Marque ao menos um item.' : 'Os itens marcados ficam como Enviada ao fornecedor.' }}
    </p>

    <section
      v-if="preview"
      class="message"
      :aria-label="`Mensagem para ${supplier.supplierName}`"
    >
      <p
        v-if="preview.error"
        role="alert"
      >
        {{ preview.error }}
      </p>
      <p
        v-else-if="preview.text === null"
        class="muted"
      >
        Carregando a mensagem…
      </p>
      <template v-else>
        <pre>{{ preview.text }}</pre>
        <BaseButton @click="emit('copy', preview.text)">
          Copiar mensagem
        </BaseButton>
      </template>
    </section>
  </article>
</template>

<style scoped>
.card {
  display: flex;
  flex-direction: column;
  gap: 12px;
  width: 100%;
  box-sizing: border-box;
  padding: 16px;
  border: 1px solid var(--enc-hairline);
  border-radius: var(--enc-radius-card);
  background: var(--enc-surface);
}
.head {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 4px 16px;
}
h2,
p {
  margin: 0;
}
h2 {
  font-size: 16px;
}
.muted {
  color: var(--enc-ink-secondary);
}
.lines {
  display: flex;
  flex-direction: column;
  gap: 12px;
  margin: 0;
  padding: 0;
  list-style: none;
}
.line {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 2px 12px;
  padding-top: 12px;
  border-top: 1px solid var(--enc-hairline);
}
.line .muted {
  flex-basis: 100%;
  padding-left: 24px;
}
.actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.hint {
  color: var(--enc-ink-secondary);
}
.hint[role='alert'] {
  color: var(--enc-status-alert-text);
  font-weight: 600;
}
[role='alert'] {
  color: var(--enc-status-alert-text);
  font-weight: 600;
}
.message {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 8px;
  padding: 12px;
  border-radius: var(--enc-radius-card);
  background: var(--enc-ground);
}
pre {
  margin: 0;
  max-width: 100%;
  font: inherit;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

/* Phone: the actions fill the width, one under the other, 48 px tall (the buttons follow --enc-control-height). */
@media (max-width: 640px) {
  .card {
    padding: 12px;
  }
  .actions > .enc-button {
    flex: 1 1 100%;
  }
  .line .muted {
    padding-left: 32px;
  }
  .line a {
    display: inline-flex;
    align-items: center;
    min-height: var(--enc-control-height);
  }
}
</style>
