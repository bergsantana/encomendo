<script setup lang="ts">
/**
 * ItemMoveMenu is the actions of one order item: one step at a time, from
 * the moves the API allows now ("Marcar como enviada ao fornecedor", "Marcar
 * como em trânsito", "Iniciar produção", "Marcar como instalada"…). On
 * receiving, "Tudo certo" or "Com defeito" (a defective item goes back);
 * "Marcar como pronto" for an in-house item asks the same. Words follow the
 * item's sourcing mode.
 */
import { BaseButton } from '@encomendo/ui'
import { ref } from 'vue'

import type { ItemCheck, ItemStatus, OrderItem } from '../labels'

const props = defineProps<{
  item: OrderItem
  busy: boolean
}>()

const emit = defineEmits<{
  move: [to: ItemStatus, check?: ItemCheck]
}>()

/** checking is true after "Marcar como pronto", while "Tudo certo" or "Com defeito" is asked. */
const checking = ref(false)

function words(to: ItemStatus): string | null {
  const inHouse = props.item.sourcingMode === 'in_house'
  const labels: Partial<Record<ItemStatus, string>> = {
    sent_to_supplier: 'Marcar como enviada ao fornecedor',
    in_transit: 'Marcar como em trânsito',
    in_production: 'Iniciar produção',
    ready: 'Marcar como pronto',
    installed: inHouse ? 'Marcar como instalado' : 'Marcar como instalada',
  }
  return labels[to] ?? null
}

function move(to: ItemStatus, check?: ItemCheck): void {
  checking.value = false
  emit('move', to, check)
}
</script>

<template>
  <div class="moves">
    <template
      v-for="to in item.nextMoves"
      :key="to"
    >
      <template v-if="to === 'received' || (to === 'ready' && checking)">
        <BaseButton
          :disabled="busy"
          :aria-label="`Tudo certo: ${item.name}`"
          @click="move(to, 'ok')"
        >
          Tudo certo
        </BaseButton>
        <BaseButton
          :disabled="busy"
          :aria-label="`Com defeito: ${item.name}`"
          @click="move(to, 'defective')"
        >
          Com defeito
        </BaseButton>
      </template>
      <BaseButton
        v-else-if="to === 'ready'"
        :disabled="busy"
        :aria-label="`${words(to)}: ${item.name}`"
        @click="checking = true"
      >
        {{ words(to) }}
      </BaseButton>
      <BaseButton
        v-else-if="words(to) !== null"
        :disabled="busy"
        :aria-label="`${words(to)}: ${item.name}`"
        @click="move(to)"
      >
        {{ words(to) }}
      </BaseButton>
    </template>
  </div>
</template>

<style scoped>
.moves {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
</style>
