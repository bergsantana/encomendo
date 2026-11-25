import { flushPromises, mount } from '@vue/test-utils'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { createPinia, setActivePinia } from 'pinia'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it } from 'vitest'

import { useHojeStore } from './store'
import HojeView from './views/HojeView.vue'

const server = setupServer()

beforeAll(() => server.listen({ onUnhandledFrame: 'error' }))
afterEach(() => server.resetHandlers())
afterAll(() => server.close())
beforeEach(() => setActivePinia(createPinia()))

const ready = () => http.get('*/readyz', () => HttpResponse.json({ status: 'ok', checks: { postgres: 'ok', redis: 'ok' } }))
const notReady = () =>
  http.get('*/readyz', () =>
    HttpResponse.json(
      { type: 'about:blank', title: 'Serviço indisponível', status: 503, code: 'not_ready' },
      { status: 503, headers: { 'Content-Type': 'application/problem+json' } },
    ),
  )

describe('useHojeStore', () => {
  it('reports ok when the API is ready', async () => {
    server.use(ready())
    const store = useHojeStore()

    await store.checkApi()

    expect(store.apiStatus).toBe('ok')
  })

  it('reports unavailable when a dependency is down', async () => {
    server.use(notReady())
    const store = useHojeStore()

    await store.checkApi()

    expect(store.apiStatus).toBe('unavailable')
  })
})

describe('HojeView', () => {
  it('checks the API on mount and shows the result in pt-BR', async () => {
    server.use(ready())
    const wrapper = mount(HojeView)

    expect(wrapper.get('[role=status]').text()).toBe('Verificando o servidor…')
    await flushPromises()

    expect(wrapper.get('h1').text()).toBe('Hoje')
    expect(wrapper.get('[role=status]').text()).toBe('Servidor conectado.')
  })

  it('tells the user when the server is unavailable', async () => {
    server.use(notReady())
    const wrapper = mount(HojeView)
    await flushPromises()

    expect(wrapper.get('[role=status]').attributes('data-status')).toBe('unavailable')
  })
})
