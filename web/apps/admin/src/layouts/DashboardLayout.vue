<script setup lang="ts">
/**
 * DashboardLayout is the management shell: menu on the left, page on the
 * right. Menu order follows the product: Hoje · Pedidos · Compras · Produção ·
 * Estoque · Catálogo · Clientes · Financeiro. Each feature adds its entry
 * when it ships; Produção and Estoque show only when the tenant turns them on.
 */
import { RouterLink, RouterView } from 'vue-router'

interface MenuItem {
  label: string
  to: { name: string }
}

const menu: MenuItem[] = [{ label: 'Hoje', to: { name: 'hoje' } }]
</script>

<template>
  <div class="layout">
    <nav
      class="menu"
      aria-label="Menu principal"
    >
      <div class="brand">
        <span
          class="mark"
          aria-hidden="true"
        />
        Encomendo
      </div>
      <RouterLink
        v-for="item in menu"
        :key="item.label"
        :to="item.to"
        class="menu-link"
        active-class="menu-link--active"
      >
        {{ item.label }}
      </RouterLink>
    </nav>
    <main class="page">
      <RouterView />
    </main>
  </div>
</template>

<style scoped>
.layout {
  display: grid;
  grid-template-columns: 220px 1fr;
  min-height: 100vh;
}
.menu {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 16px 12px;
  background: var(--enc-surface);
  border-right: 1px solid var(--enc-hairline);
}
.brand {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 16px;
  font-weight: 600;
}
.mark {
  width: 20px;
  height: 20px;
  border-radius: 6px;
  background: var(--enc-accent);
}
.menu-link {
  padding: 8px 10px;
  border-radius: var(--enc-radius-card);
  color: var(--enc-ink);
  text-decoration: none;
}
.menu-link--active {
  box-shadow: inset 3px 0 0 var(--enc-accent);
  background: var(--enc-ground);
  font-weight: 600;
}
.page {
  padding: 24px 32px;
}
</style>
