import { createRouter, createWebHistory, type Router, type RouteRecordRaw, type RouterHistory } from 'vue-router'

import DashboardLayout from '../layouts/DashboardLayout.vue'
import { createAuthGuard } from './guards'

/**
 * Routes are added by each feature as it lands. `requiresAuth` and
 * `requiresStore` on the dashboard protect every screen inside it;
 * `permission` on a screen only hides it, since the API checks every call
 * again.
 */
export const routes: RouteRecordRaw[] = [
  {
    path: '/entrar',
    name: 'login',
    component: () => import('../features/auth/views/LoginView.vue'),
    meta: { guestOnly: true },
  },
  {
    path: '/criar-conta',
    name: 'signup',
    component: () => import('../features/auth/views/SignUpView.vue'),
    meta: { guestOnly: true },
  },
  {
    path: '/lojas',
    name: 'stores',
    component: () => import('../features/auth/views/StorePickerView.vue'),
    meta: { requiresAuth: true },
  },
  {
    path: '/',
    component: DashboardLayout,
    meta: { requiresAuth: true, requiresStore: true },
    children: [
      { path: '', redirect: { name: 'hoje' } },
      { path: 'hoje', name: 'hoje', component: () => import('../features/hoje/views/HojeView.vue') },
    ],
  },
  { path: '/:pathMatch(.*)*', redirect: { name: 'hoje' } },
]

/** createAppRouter builds the admin router with the auth guard. Tests pass a memory history. */
export function createAppRouter(history: RouterHistory = createWebHistory()): Router {
  const router = createRouter({ history, routes })
  router.beforeEach(createAuthGuard({ login: 'login', home: 'hoje', storePicker: 'stores' }))
  return router
}
