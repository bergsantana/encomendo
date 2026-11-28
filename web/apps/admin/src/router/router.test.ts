import { flushPromises, mount } from '@vue/test-utils'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { createPinia, setActivePinia } from 'pinia'
import { afterAll, afterEach, beforeAll, describe, expect, it } from 'vitest'
import { createMemoryHistory } from 'vue-router'

import App from '../App.vue'
import { authSession, problem } from '../testing/fixtures'
import { todayHandler } from '../testing/today'
import { createAppRouter } from '.'

const server = setupServer(todayHandler())
beforeAll(() => server.listen({ onUnhandledFrame: 'error' }))
afterEach(() => server.resetHandlers())
afterAll(() => server.close())

const signedIn = () => server.use(http.post('*/auth/refresh', () => HttpResponse.json(authSession())))
const signedOut = () => server.use(http.post('*/auth/refresh', () => problem(401, 'unauthenticated', 'Sessão expirada')))

async function mountAt(path: string) {
  const pinia = createPinia()
  setActivePinia(pinia)
  const router = createAppRouter(createMemoryHistory())
  await router.push(path)
  const wrapper = mount(App, { global: { plugins: [pinia, router] } })
  await flushPromises()
  return { wrapper, router }
}

describe('admin routes', () => {
  it('opens Hoje from the root once signed in', async () => {
    signedIn()

    const { wrapper, router } = await mountAt('/')

    expect(router.currentRoute.value.name).toBe('hoje')
    expect(wrapper.get('h1').text()).toBe('Hoje')
    expect(wrapper.get('nav').text()).toContain('Hoje')
  })

  it('sends unknown paths to Hoje', async () => {
    signedIn()

    const { router } = await mountAt('/nao-existe')

    expect(router.currentRoute.value.name).toBe('hoje')
  })

  it('keeps the management screens behind login', async () => {
    signedOut()

    const { router } = await mountAt('/hoje')

    expect(router.currentRoute.value.name).toBe('login')
    expect(router.currentRoute.value.query).toEqual({ redirect: '/hoje' })
  })
})
