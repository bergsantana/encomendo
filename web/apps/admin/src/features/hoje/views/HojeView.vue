<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { computed, onMounted } from 'vue'

import { useHojeStore } from '../store'

const store = useHojeStore()
const { apiStatus } = storeToRefs(store)

const statusText = computed(
  () =>
    ({
      checking: 'Verificando o servidor…',
      ok: 'Servidor conectado.',
      unavailable: 'Servidor indisponível. Tente novamente em instantes.',
    })[apiStatus.value],
)

onMounted(() => store.checkApi())
</script>

<template>
  <section>
    <h1>Hoje</h1>
    <p
      role="status"
      :data-status="apiStatus"
    >
      {{ statusText }}
    </p>
  </section>
</template>
