import type { Permission } from '@encomendo/api'
import { flushPromises, mount } from '@vue/test-utils'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { createPinia, setActivePinia } from 'pinia'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it } from 'vitest'
import { defineComponent, h } from 'vue'
import { createMemoryHistory, createRouter, RouterView, type RouteRecordRaw } from 'vue-router'

import { authSession, problem } from '../testing/fixtures'
import { createAuthGuard } from './guards'

const page = (title: string) => defineComponent({ render: () => h('h1', title) })

const routes: RouteRecordRaw[] = [
  { path: '/entrar', name: 'login', component: page('Entrar'), meta: { guestOnly: true } },
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
    path: '/ajustes',
    component: RouterView,
    meta: { requiresAuth: true, permission: 'settings:read' },
    children: [{ path: 'loja', name: 'store-settings', component: page('Dados da loja'), meta: { permission: 'settings:write' } }],
  },
]

const server = setupServer()
beforeAll(() => server.listen({ onUnhandledFrame: 'error' }))
afterEach(() => server.resetHandlers())
afterAll(() => server.close())
beforeEach(() => setActivePinia(createPinia()))

/** The refresh cookie, as the server sees it: a session with these permissions, or none. */
const cookieFor = (permissions: Permission[] | null) =>
  server.use(
    http.post('*/auth/refresh', () =>
      permissions === null ? problem(401, 'unauthenticated', 'Sessão expirada') : HttpResponse.json(authSession({ permissions })),
    ),
  )

/** open loads the app at a path, as a reload or a pasted link does. */
async function open(path: string) {
  const router = createRouter({ history: createMemoryHistory(), routes })
  router.beforeEach(createAuthGuard({ login: 'login', home: 'hoje' }))
  await router.push(path)
  const wrapper = mount(RouterView, { global: { plugins: [router] } })
  await flushPromises()
  return { router, heading: wrapper.get('h1').text() }
}

describe('createAuthGuard', () => {
  it('sends a signed-out user to login and remembers where they were going', async () => {
    cookieFor(null)

    const { router, heading } = await open('/financeiro?mes=2026-11')

    expect(heading).toBe('Entrar')
    expect(router.currentRoute.value.query).toEqual({ redirect: '/financeiro?mes=2026-11' })
  })

  it('restores the session from the cookie before deciding, so a reload keeps the page', async () => {
    cookieFor(['finance:read'])

    const { heading } = await open('/financeiro')

    expect(heading).toBe('Financeiro')
  })

  it('protects every child of a route that requires auth', async () => {
    cookieFor(null)

    const { heading } = await open('/')

    expect(heading).toBe('Entrar')
  })

  it('sends a signed-out user to login from a route that only names a permission', async () => {
    cookieFor(null)

    const { heading } = await open('/relatorios')

    expect(heading).toBe('Entrar')
  })

  it('opens public routes for anyone', async () => {
    cookieFor(null)

    const { heading } = await open('/convite')

    expect(heading).toBe('Aceitar convite')
  })

  it('never copies a URL fragment into the login link, where servers would log it', async () => {
    // Invitation links carry their token in the fragment so it never reaches a server log.
    cookieFor(null)

    const { router } = await open('/financeiro?mes=2026-11#token=secret')

    expect(router.currentRoute.value.query).toEqual({ redirect: '/financeiro?mes=2026-11' })
    expect(router.currentRoute.value.fullPath).not.toContain('secret')
  })

  it('stays on login while the server cannot answer, and asks again on the next navigation', async () => {
    let calls = 0
    server.use(
      http.post('*/auth/refresh', () => {
        calls += 1
        return problem(503, 'not_ready', 'Serviço indisponível')
      }),
    )

    const { router, heading } = await open('/financeiro')
    await router.push('/convite')

    expect(heading).toBe('Entrar')
    expect(calls).toBe(2)
  })

  it('sends a signed-in member away from login, to where they were going', async () => {
    cookieFor(['finance:read'])

    const { heading } = await open('/entrar?redirect=/financeiro')

    expect(heading).toBe('Financeiro')
  })

  it('sends a signed-in member away from login, to the home screen when there is nowhere else', async () => {
    cookieFor(['order:read'])

    const { heading } = await open('/entrar?redirect=https://evil.example')

    expect(heading).toBe('Hoje')
  })

  it('sends a member whose role lacks the permission to the home screen', async () => {
    cookieFor(['order:read'])

    const { router, heading } = await open('/financeiro')

    expect(heading).toBe('Hoje')
    expect(router.currentRoute.value.name).toBe('hoje')
  })

  it("requires the parent's permission as well as the child's", async () => {
    cookieFor(['settings:write'])

    const { heading } = await open('/ajustes/loja')

    expect(heading).toBe('Hoje')
  })

  it('opens a nested screen when the role holds every permission on the way', async () => {
    cookieFor(['settings:read', 'settings:write'])

    const { heading } = await open('/ajustes/loja')

    expect(heading).toBe('Dados da loja')
  })
})
