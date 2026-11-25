import { flushPromises, mount } from '@vue/test-utils'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { createPinia } from 'pinia'
import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'

import App from '../App.vue'
import { routes } from '.'

const server = setupServer(http.get('*/readyz', () => HttpResponse.json({ status: 'ok' })))
beforeAll(() => server.listen({ onUnhandledFrame: 'error' }))
afterAll(() => server.close())

async function mountAt(path: string) {
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push(path)
  const wrapper = mount(App, { global: { plugins: [createPinia(), router] } })
  await flushPromises()
  return { wrapper, router }
}

describe('admin routes', () => {
  it('opens Hoje from the root', async () => {
    const { wrapper, router } = await mountAt('/')

    expect(router.currentRoute.value.name).toBe('hoje')
    expect(wrapper.get('h1').text()).toBe('Hoje')
    expect(wrapper.get('nav').text()).toContain('Hoje')
  })

  it('sends unknown paths to Hoje', async () => {
    const { router } = await mountAt('/nao-existe')

    expect(router.currentRoute.value.name).toBe('hoje')
  })
})
