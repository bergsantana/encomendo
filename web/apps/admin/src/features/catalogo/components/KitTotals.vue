<script setup lang="ts">
/**
 * KitTotals is the totals block of the kit editor. Every figure is the one
 * the API sent with the kit as it was last saved; nothing is added up here
 * (ADR 0025). While the form holds edits the figures are stale, and the block
 * says so. "Margem" shows only when the API sent it, which is to the owner.
 */
import type { Kit } from '@encomendo/api'
import { computed } from 'vue'

import { formatDays, formatMargin, formatMoney } from '../../../format'
import { niche } from '../../../niche'

const props = defineProps<{
  /** kit is the kit as last saved, or null for one that was never saved. */
  kit: Kit | null
  /** edited is true while the form differs from that kit. */
  edited: boolean
}>()

const margin = computed(() => {
  const kit = props.kit
  if (kit === null || kit.margin === undefined) return null
  return kit.margin === null ? '—' : formatMargin(kit.margin, kit.price.currency)
})
</script>

<template>
  <section
    class="totals"
    aria-label="Totais do kit"
  >
    <p
      v-if="kit === null"
      class="hint"
    >
      Os totais e o prazo aparecem depois de salvar.
    </p>
    <template v-else>
      <p
        v-if="edited"
        class="hint"
      >
        Os totais e o prazo mostram o último salvamento. Eles se atualizam depois de salvar.
      </p>
      <p>Soma dos itens avulsos: <strong class="enc-number">{{ formatMoney(kit.singleItemsTotal.cents, kit.singleItemsTotal.currency) }}</strong></p>
      <p>Desconto do kit: <strong class="enc-number">{{ formatMoney(kit.discount.cents, kit.discount.currency) }}</strong></p>
      <p v-if="margin !== null">
        Margem: <strong class="enc-number">{{ margin }}</strong>
      </p>
      <p>
        Prazo calculado: <strong class="enc-number">{{ formatDays(kit.days.total) }}</strong>
        (item mais lento {{ kit.days.slowestItem }} + {{ niche.montagem }} {{ kit.days.assembly }} + margem de segurança {{ kit.days.safetyMargin }})
      </p>
      <p
        v-if="kit.discount.cents < 0"
        role="status"
        class="warning"
      >
        O preço do kit está acima da soma avulsa.
      </p>
    </template>
  </section>
</template>

<style scoped>
.totals {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 12px;
  border: 1px solid var(--enc-hairline);
  border-radius: var(--enc-radius-card);
}
p {
  margin: 0;
}
.hint {
  color: var(--enc-ink-secondary);
}
.warning {
  font-weight: 600;
  color: var(--enc-status-alert-text);
}
</style>
