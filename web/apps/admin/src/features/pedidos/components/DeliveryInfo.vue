<script setup lang="ts">
/**
 * DeliveryInfo is "Entrega": the promised date ("Entrega prevista"), "Precisa
 * chegar até", and how the order is handed over, with "Alterar data" for the
 * roles that may change the promise. The dates are the API's.
 */
import type { Order } from '@encomendo/api'
import { BaseButton } from '@encomendo/ui'

import { formatCivilDate } from '../../../format'

defineProps<{
  order: Order
  canChangeDate: boolean
}>()

const emit = defineEmits<{
  changeDate: []
}>()

function addressText(order: Order): string {
  const address = order.address
  if (address === null) return ''
  const place = [address.street, address.number].filter((part) => part !== '').join(', ')
  return [place, address.district, `${address.city}/${address.state}`].filter((part) => part !== '').join(' - ')
}
</script>

<template>
  <section
    class="delivery"
    aria-labelledby="delivery-heading"
  >
    <h2 id="delivery-heading">
      Entrega
    </h2>
    <p>Entrega prevista: <strong class="enc-number">{{ formatCivilDate(order.deliveryDate) }}</strong></p>
    <p>Precisa chegar até: <strong class="enc-number">{{ formatCivilDate(order.arrivalLimit) }}</strong></p>
    <p>{{ order.handover === 'pickup' ? 'Retirada na loja' : (order.address === null ? 'Entrega' : `Entrega em ${addressText(order)}`) }}</p>
    <div v-if="canChangeDate">
      <BaseButton @click="emit('changeDate')">
        Alterar data
      </BaseButton>
    </div>
  </section>
</template>

<style scoped>
.delivery {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
h2,
p {
  margin: 0;
}
h2 {
  font-size: 15px;
}
.delivery > div {
  margin-top: 4px;
}
</style>
