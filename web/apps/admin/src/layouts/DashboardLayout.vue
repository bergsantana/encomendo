<script setup lang="ts">
/**
 * DashboardLayout is the management shell: menu on the left, page on the
 * right. Menu order follows the product: Hoje · Pedidos · Compras · Produção ·
 * Estoque · Catálogo · Clientes · Financeiro. Each feature adds its entry
 * when it ships; Produção and Estoque show only when the tenant turns them on.
 * The foot of the menu shows who is signed in, in which store, and logs out;
 * App leaves for login once the session has ended.
 */
import { BaseButton } from '@encomendo/ui'
import { storeToRefs } from 'pinia'
import { RouterLink, RouterView } from 'vue-router'

import { useAuthStore } from '../features/auth/store'
import { useSessionStore } from '../stores/session'

interface MenuItem {
  label: string
  to: { name: string }
}

const menu: MenuItem[] = [{ label: 'Hoje', to: { name: 'hoje' } }]

const { user, tenant, memberships } = storeToRefs(useSessionStore())
const auth = useAuthStore()
const { loggingOut, logOutError } = storeToRefs(auth)
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
      <p
        v-if="tenant"
        class="store"
      >
        {{ tenant.name }}
      </p>
      <RouterLink
        v-for="item in menu"
        :key="item.label"
        :to="item.to"
        class="menu-link"
        active-class="menu-link--active"
      >
        {{ item.label }}
      </RouterLink>
      <div class="account">
        <span class="account-name">{{ user?.name }}</span>
        <RouterLink
          v-if="memberships.length > 1"
          :to="{ name: 'stores' }"
          class="account-link"
        >
          Trocar de loja
        </RouterLink>
        <BaseButton
          :disabled="loggingOut"
          @click="auth.logOut()"
        >
          Sair
        </BaseButton>
        <p
          v-if="logOutError"
          role="alert"
          class="account-error"
        >
          {{ logOutError }}
        </p>
      </div>
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
  margin-bottom: 4px;
  font-weight: 600;
}
.mark {
  width: 20px;
  height: 20px;
  border-radius: 6px;
  background: var(--enc-accent);
}
.store {
  margin: 0 0 12px;
  color: var(--enc-ink-secondary);
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
.account {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-top: auto;
  padding-top: 16px;
  border-top: 1px solid var(--enc-hairline);
}
.account-name {
  font-weight: 600;
}
.account-error {
  margin: 0;
}
.account-link {
  color: var(--enc-ink);
}
.page {
  padding: 24px 32px;
}
</style>
