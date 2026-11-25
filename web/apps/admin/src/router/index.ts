import { createRouter, createWebHistory, type Router, type RouteRecordRaw, type RouterHistory } from 'vue-router'

import DashboardLayout from '../layouts/DashboardLayout.vue'
import { createAuthGuard } from './guards'

/**
 * Routes are added by each feature as it lands. `requiresAuth` on the
 * dashboard protects every screen inside it; `permission` on a screen only
 * hides it, since the API checks every call again.
 */
export const routes: RouteRecordRaw[] = [
  {
    path: '/entrar',
    name: 'login',
    component: () => import('../features/auth/views/LoginView.vue'),
    meta: { guestOnly: true },
  },
  {
    path: '/',
    component: DashboardLayout,
    meta: { requiresAuth: true },
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
  router.beforeEach(createAuthGuard({ login: 'login', home: 'hoje' }))
  return router
}
