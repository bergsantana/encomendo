import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'

import DashboardLayout from '../layouts/DashboardLayout.vue'

/**
 * Routes are added by each feature as it lands. Route meta will carry
 * `requiresAuth` and `permission` from milestone 2; permissions only hide UI.
 */
export const routes: RouteRecordRaw[] = [
  {
    path: '/',
    component: DashboardLayout,
    children: [
      { path: '', redirect: { name: 'hoje' } },
      { path: 'hoje', name: 'hoje', component: () => import('../features/hoje/views/HojeView.vue') },
    ],
  },
  { path: '/:pathMatch(.*)*', redirect: { name: 'hoje' } },
]

export const router = createRouter({ history: createWebHistory(), routes })
