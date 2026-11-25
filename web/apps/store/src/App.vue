<script setup lang="ts">
/**
 * App boots the right screen for the host: a store page, the client area, or
 * a not-found page. The store sections (Kits prontos, Monte o seu,
 * Acompanhar pedido) and the client area land in milestone 7.
 */
import type { Site } from './site'

defineProps<{ site: Site }>()
</script>

<template>
  <main class="page">
    <template v-if="site.kind === 'store'">
      <h1>Loja {{ site.slug }}</h1>
      <p>Em breve: kits prontos, Monte o seu e acompanhamento de pedidos.</p>
    </template>
    <template v-else-if="site.kind === 'account'">
      <h1>Minhas encomendas</h1>
      <p>Em breve: todas as suas encomendas, de todas as lojas, em um só lugar.</p>
    </template>
    <template v-else>
      <h1>Loja não encontrada</h1>
      <p>Confira o endereço que você recebeu da loja.</p>
    </template>
  </main>
</template>

<style scoped>
.page {
  max-width: 560px;
  margin: 0 auto;
  padding: 24px 16px;
}
</style>
