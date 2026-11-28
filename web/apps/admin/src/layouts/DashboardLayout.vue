<script setup lang="ts">
/**
 * DashboardLayout is the management shell: menu on the left, page on the
 * right. Menu order follows the product: Hoje · Pedidos · Compras · Produção ·
 * Estoque · Catálogo · Clientes · Financeiro. Each feature adds its entry
 * when it ships; Produção and Estoque show only when the tenant turns them on.
 * Equipe comes after them, for the roles that manage the team, then
 * Configurações for the owner. The foot of
 * the menu shows who is signed in, in which store, and logs out; App leaves
 * for login once the session has ended.
 *
 * On phones (640 px and narrower) the menu is a drawer behind the "Menu"
 * button of a top bar. CSS does the layout; the script only keeps the drawer
 * honest for the keyboard: focus goes in, Tab stays in, Escape closes and
 * gives focus back to the button, and choosing an entry closes it and moves
 * focus to the page.
 */
import type { Permission } from '@encomendo/api'
import { BaseButton, IconMenu2, IconX, trapTab, useIsPhone } from '@encomendo/ui'
import { storeToRefs } from 'pinia'
import { computed, nextTick, onBeforeUnmount, ref, useTemplateRef, watch } from 'vue'
import { RouterLink, RouterView, useRoute } from 'vue-router'

import { useAuthStore } from '../features/auth/store'
import { useSessionStore } from '../stores/session'

interface MenuItem {
  label: string
  to: { name: string }
  /** alsoOn lists other routes of the same screen, such as a customer's own address, that keep the entry marked. */
  alsoOn?: string[]
  /** permission hides the entry from roles without it; the route and the API check it again. */
  permission?: Permission
}

const items: MenuItem[] = [
  { label: 'Hoje', to: { name: 'hoje' } },
  { label: 'Pedidos', to: { name: 'orders' }, alsoOn: ['order', 'new-order', 'change-order'], permission: 'order:read' },
  { label: 'Catálogo', to: { name: 'catalog' }, permission: 'catalog:read' },
  { label: 'Clientes', to: { name: 'customers' }, alsoOn: ['customer'], permission: 'order:read' },
  { label: 'Financeiro', to: { name: 'finance' }, permission: 'finance:read' },
  { label: 'Equipe', to: { name: 'members' }, permission: 'members:manage' },
  { label: 'Configurações', to: { name: 'settings' }, permission: 'settings:write' },
]

const session = useSessionStore()
const { user, tenant, memberships } = storeToRefs(session)
const menu = computed(() => items.filter((item) => item.permission === undefined || session.can(item.permission)))
const auth = useAuthStore()
const { loggingOut, logOutError } = storeToRefs(auth)

const drawerOpen = ref(false)
const menuButton = useTemplateRef<HTMLButtonElement>('menuButton')
const closeButton = useTemplateRef<HTMLButtonElement>('closeButton')
const drawer = useTemplateRef<HTMLElement>('drawer')
const page = useTemplateRef<HTMLElement>('page')
const route = useRoute()

/** isOnScreen marks an entry whose screen has more than one address. */
function isOnScreen(item: MenuItem): boolean {
  return item.alsoOn?.includes(String(route.name)) ?? false
}
const isPhone = useIsPhone()

async function openDrawer(): Promise<void> {
  drawerOpen.value = true
  await nextTick()
  closeButton.value?.focus()
}

/** closeDrawer shuts the drawer; focus goes back to the button that opened it, or to the page after a choice. */
async function closeDrawer(focus: 'button' | 'page' | 'none'): Promise<void> {
  if (!drawerOpen.value) return
  drawerOpen.value = false
  await nextTick()
  if (focus === 'button') menuButton.value?.focus()
  else if (focus === 'page') page.value?.focus()
}

// On the document, not the drawer: a tap on the scrim or the page can leave focus on <body>,
// and Escape and Tab must still work then.
function onKeydown(event: KeyboardEvent): void {
  if (event.key === 'Escape') {
    event.stopPropagation()
    void closeDrawer('button')
    return
  }
  const menuEl = drawer.value
  if (event.key !== 'Tab' || menuEl === null) return
  if (menuEl.contains(document.activeElement)) {
    trapTab(event, menuEl)
    return
  }
  event.preventDefault()
  closeButton.value?.focus()
}

// Choosing an entry closes the drawer, also the page already open, which changes no route.
function onDrawerClick(event: MouseEvent): void {
  if (event.target instanceof Element && event.target.closest('a') !== null) void closeDrawer('page')
}

watch(drawerOpen, (open) => {
  if (open) document.addEventListener('keydown', onKeydown)
  else document.removeEventListener('keydown', onKeydown)
})
onBeforeUnmount(() => document.removeEventListener('keydown', onKeydown))

watch(
  () => route.fullPath,
  () => closeDrawer('page'),
)
// A phone turned sideways, or a window widened, shows the full menu: nothing stays open behind it.
watch(isPhone, (phone) => {
  if (!phone) void closeDrawer('none')
})
</script>

<template>
  <div
    class="layout"
    :class="{ 'layout--open': drawerOpen }"
  >
    <!-- inert is set or removed, never "false": an older WebView without the property would read the bare attribute as inert. -->
    <header
      class="topbar"
      :inert="drawerOpen ? true : undefined"
    >
      <button
        ref="menuButton"
        type="button"
        class="topbar__button"
        aria-controls="main-menu"
        :aria-expanded="drawerOpen"
        @click="openDrawer"
      >
        <IconMenu2
          :size="20"
          aria-hidden="true"
        />
        Menu
      </button>
      <span
        v-if="tenant"
        class="topbar__store"
      >{{ tenant.name }}</span>
    </header>
    <div
      class="scrim"
      data-testid="menu-scrim"
      aria-hidden="true"
      @click="closeDrawer('button')"
    />
    <nav
      id="main-menu"
      ref="drawer"
      class="menu"
      aria-label="Menu principal"
      @click="onDrawerClick"
    >
      <div class="brand">
        <span
          class="mark"
          aria-hidden="true"
        />
        Encomendo
        <button
          ref="closeButton"
          type="button"
          class="menu__close"
          @click="closeDrawer('button')"
        >
          <IconX
            :size="20"
            aria-hidden="true"
          />
          Fechar menu
        </button>
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
        :class="{ 'menu-link--active': isOnScreen(item) }"
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
    <main
      ref="page"
      class="page"
      tabindex="-1"
      :inert="drawerOpen ? true : undefined"
    >
      <RouterView />
    </main>
  </div>
</template>

<style scoped>
.layout {
  display: grid;
  grid-template-columns: 220px minmax(0, 1fr);
  min-height: 100vh;
}
.topbar,
.scrim,
.menu__close {
  display: none;
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
.page:focus {
  outline: none;
}

/* Phone: a top bar with the "Menu" button, and the menu as a drawer from the left. */
@media (max-width: 640px) {
  .layout {
    grid-template-columns: minmax(0, 1fr);
    grid-template-rows: auto 1fr;
  }
  .topbar {
    position: sticky;
    top: 0;
    z-index: 5;
    display: flex;
    align-items: center;
    gap: 12px;
    min-height: 56px;
    padding: 4px 8px;
    background: var(--enc-surface);
    border-bottom: 1px solid var(--enc-hairline);
  }
  .topbar__button,
  .menu__close {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    height: var(--enc-control-height);
    padding: 0 12px;
    border: 1px solid var(--enc-secondary-border);
    border-radius: var(--enc-radius-card);
    background: var(--enc-surface);
    color: var(--enc-ink);
    font: inherit;
    font-weight: 600;
    cursor: pointer;
  }
  .topbar__store {
    min-width: 0;
    overflow: hidden;
    font-weight: 600;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .menu {
    position: fixed;
    inset: 0 auto 0 0;
    z-index: 20;
    display: none;
    box-sizing: border-box;
    width: min(300px, 85vw);
    padding: 16px;
    overflow-y: auto;
    box-shadow: 0 4px 24px rgb(27 29 32 / 20%);
  }
  .layout--open .menu {
    display: flex;
  }
  .layout--open .scrim {
    position: fixed;
    inset: 0;
    z-index: 15;
    display: block;
    background: rgb(27 29 32 / 40%);
  }
  .brand {
    margin-bottom: 12px;
  }
  .menu__close {
    margin-left: auto;
  }
  .menu-link,
  .account-link {
    display: flex;
    align-items: center;
    min-height: var(--enc-control-height);
    padding: 0 10px;
  }
  .page {
    padding: 16px;
  }
}
</style>
