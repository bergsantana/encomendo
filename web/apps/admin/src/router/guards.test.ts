import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it } from 'vitest'
import { defineComponent, h } from 'vue'
import { createMemoryHistory, createRouter, RouterView, type RouteRecordRaw } from 'vue-router'

import { useSessionStore } from '../stores/session'
import { createAuthGuard } from './guards'

const page = (title: string) => defineComponent({ render: () => h('h1', title) })

const routes: RouteRecordRaw[] = [
  { path: '/entrar', name: 'login', component: page('Entrar') },
  { path: '/convite', name: 'invite', component: page('Aceitar convite') },
  {
    path: '/',
    component: RouterView,
    meta: { requiresAuth: true },
    children: [
      { path: '', name: 'hoje', component: page('Hoje') },
      { path: 'financeiro', name: 'financeiro', component: page('Financeiro'), meta: { permission: 'finance:read' } },
    ],
  },
  { path: '/relatorios', name: 'reports', component: page('Relatórios'), meta: { permission: 'finance:read' } },
  {
    path: '/caixa',
    component: RouterView,
    meta: { requiresAuth: true, permission: 'finance:read' },
    children: [{ path: 'lancar', name: 'entry', component: page('Novo lançamento'), meta: { permission: 'finance:write' } }],
  },
]

beforeEach(() => setActivePinia(createPinia()))

async function open(path: string) {
  const router = createRouter({ history: createMemoryHistory(), routes })
  router.beforeEach(createAuthGuard({ login: 'login', home: 'hoje' }))
  await router.push(path)
  const wrapper = mount(RouterView, { global: { plugins: [router] } })
  await flushPromises()
  return { router, heading: wrapper.get('h1').text() }
}

const signIn = (permissions: string[]) => useSessionStore().start({ accessToken: 't', permissions })

describe('createAuthGuard', () => {
  it('sends a signed-out user to login and remembers where they were going', async () => {
    const { router, heading } = await open('/financeiro?mes=2026-11')

    expect(heading).toBe('Entrar')
    expect(router.currentRoute.value.query).toEqual({ redirect: '/financeiro?mes=2026-11' })
  })

  it('protects every child of a route that requires auth', async () => {
    const { heading } = await open('/')

    expect(heading).toBe('Entrar')
  })

  it('sends a signed-out user to login from a route that only names a permission', async () => {
    const { heading } = await open('/relatorios')

    expect(heading).toBe('Entrar')
  })

  it('opens public routes for anyone', async () => {
    const { heading } = await open('/convite')

    expect(heading).toBe('Aceitar convite')
  })

  it('lets a signed-in member open a screen that needs no permission', async () => {
    signIn(['order:read'])

    const { heading } = await open('/')

    expect(heading).toBe('Hoje')
  })

  it('opens a screen when the role holds its permission', async () => {
    signIn(['order:read', 'finance:read'])

    const { heading } = await open('/financeiro')

    expect(heading).toBe('Financeiro')
  })

  it('sends a member whose role lacks the permission to the home screen', async () => {
    signIn(['order:read'])

    const { router, heading } = await open('/financeiro')

    expect(heading).toBe('Hoje')
    expect(router.currentRoute.value.name).toBe('hoje')
  })

  it("requires the parent's permission as well as the child's", async () => {
    signIn(['finance:write'])

    const { heading } = await open('/caixa/lancar')

    expect(heading).toBe('Hoje')
  })

  it('opens a nested screen when the role holds every permission on the way', async () => {
    signIn(['finance:read', 'finance:write'])

    const { heading } = await open('/caixa/lancar')

    expect(heading).toBe('Novo lançamento')
  })
})
