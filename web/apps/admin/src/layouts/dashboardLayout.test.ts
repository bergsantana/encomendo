import { enableAutoUnmount, flushPromises } from '@vue/test-utils'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'

import { button, closeApps, link, openApp } from '../testing/app'
import { altaRpm, authSession } from '../testing/fixtures'

// The phone layout itself is CSS; these tests cover what only script does for
// the menu drawer: where focus goes, what Tab and Escape do, what closes it.
const server = setupServer()
beforeAll(() => server.listen({ onUnhandledFrame: 'error' }))
afterEach(() => server.resetHandlers())
afterAll(() => server.close())
enableAutoUnmount(afterEach)
afterEach(closeApps)

const otherStore = { id: '0190f3c4-5e1a-7b2c-8d3e-4f5a6b7c8d01', name: 'Outra Loja', slug: 'outra' }

beforeEach(() =>
  server.use(
    http.get('*/readyz', () => HttpResponse.json({ status: 'ok' })),
    http.post('*/auth/refresh', () =>
      HttpResponse.json(
        authSession({
          permissions: ['order:read', 'members:manage'],
          memberships: [
            { tenant: altaRpm, role: 'owner', status: 'active' },
            { tenant: otherStore, role: 'admin', status: 'active' },
          ],
        }),
      ),
    ),
    http.get('*/api/v1/members', () => HttpResponse.json({ items: [], nextCursor: null })),
    http.get('*/api/v1/invitations', () => HttpResponse.json({ items: [], nextCursor: null })),
  ),
)

const menuButton = (wrapper: Awaited<ReturnType<typeof openApp>>['wrapper']) => button(wrapper, 'Menu')

describe('the menu drawer', () => {
  it('opens behind the Menu button, with focus on its close button and the page out of reach', async () => {
    const { wrapper } = await openApp('/hoje')
    expect(menuButton(wrapper).attributes('aria-expanded')).toBe('false')

    await menuButton(wrapper).trigger('click')

    expect(menuButton(wrapper).attributes('aria-expanded')).toBe('true')
    expect(document.activeElement).toBe(button(wrapper, 'Fechar menu').element)
    expect(wrapper.get('main').attributes('inert')).toBeDefined()
    expect(wrapper.get('header').attributes('inert')).toBeDefined()
    // What the menu always holds: the store, the person, the switch and the way out.
    const menu = wrapper.get('nav[aria-label="Menu principal"]')
    expect(menu.text()).toContain('Alta RPM')
    expect(menu.text()).toContain('Ana Souza')
    expect(link(wrapper, 'Trocar de loja').exists()).toBe(true)
    expect(button(wrapper, 'Sair').exists()).toBe(true)
  })

  it('keeps Tab inside, in both directions', async () => {
    const { wrapper } = await openApp('/hoje')
    await menuButton(wrapper).trigger('click')
    const menu = wrapper.get('nav[aria-label="Menu principal"]')
    const close = button(wrapper, 'Fechar menu').element
    const last = button(wrapper, 'Sair').element

    last.focus()
    await menu.trigger('keydown', { key: 'Tab' })
    expect(document.activeElement).toBe(close)

    await menu.trigger('keydown', { key: 'Tab', shiftKey: true })
    expect(document.activeElement).toBe(last)
  })

  it('closes on Escape and gives focus back to the Menu button', async () => {
    const { wrapper } = await openApp('/hoje')
    await menuButton(wrapper).trigger('click')

    await wrapper.get('nav[aria-label="Menu principal"]').trigger('keydown', { key: 'Escape' })
    await flushPromises()

    expect(menuButton(wrapper).attributes('aria-expanded')).toBe('false')
    expect(document.activeElement).toBe(menuButton(wrapper).element)
    expect(wrapper.get('main').attributes('inert')).toBeUndefined()
    expect(wrapper.get('header').attributes('inert')).toBeUndefined()
  })

  it('still answers Escape and Tab when focus fell to the page body', async () => {
    const { wrapper } = await openApp('/hoje')
    await menuButton(wrapper).trigger('click')
    ;(document.activeElement as HTMLElement).blur()
    expect(document.activeElement).toBe(document.body)

    const tab = new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true })
    document.body.dispatchEvent(tab)
    expect(tab.defaultPrevented).toBe(true)
    expect(document.activeElement).toBe(button(wrapper, 'Fechar menu').element)

    ;(document.activeElement as HTMLElement).blur()
    document.body.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await flushPromises()

    expect(menuButton(wrapper).attributes('aria-expanded')).toBe('false')
    expect(document.activeElement).toBe(menuButton(wrapper).element)
  })

  it('stops listening to the keyboard once closed', async () => {
    const { wrapper } = await openApp('/hoje')
    await menuButton(wrapper).trigger('click')
    await button(wrapper, 'Fechar menu').trigger('click')
    menuButton(wrapper).element.blur()

    const tab = new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true })
    document.body.dispatchEvent(tab)

    expect(tab.defaultPrevented).toBe(false)
  })

  it('closes with Fechar menu and with a tap outside', async () => {
    const { wrapper } = await openApp('/hoje')
    await menuButton(wrapper).trigger('click')
    await button(wrapper, 'Fechar menu').trigger('click')
    expect(menuButton(wrapper).attributes('aria-expanded')).toBe('false')

    await menuButton(wrapper).trigger('click')
    await wrapper.get('[data-testid=menu-scrim]').trigger('click')
    expect(menuButton(wrapper).attributes('aria-expanded')).toBe('false')
    expect(document.activeElement).toBe(menuButton(wrapper).element)
  })

  it('closes after choosing an entry, and focus moves to the page', async () => {
    const { wrapper, router } = await openApp('/hoje')
    await menuButton(wrapper).trigger('click')

    await link(wrapper, 'Equipe').trigger('click')
    await vi.waitFor(() => expect(router.currentRoute.value.name).toBe('members'))
    await flushPromises()

    expect(menuButton(wrapper).attributes('aria-expanded')).toBe('false')
    expect(document.activeElement).toBe(wrapper.get('main').element)
  })

  it('closes when the page already open is chosen again', async () => {
    const { wrapper } = await openApp('/hoje')
    await menuButton(wrapper).trigger('click')

    await link(wrapper, 'Hoje').trigger('click')
    await flushPromises()

    expect(menuButton(wrapper).attributes('aria-expanded')).toBe('false')
    expect(document.activeElement).toBe(wrapper.get('main').element)
  })

  it('closes by itself when the screen grows past the phone layout', async () => {
    const listeners: ((event: { matches: boolean }) => void)[] = []
    window.matchMedia = ((query: string) => ({
      matches: true,
      media: query,
      addEventListener: (_: string, listener: (event: { matches: boolean }) => void) => listeners.push(listener),
      removeEventListener: () => {},
    })) as unknown as typeof window.matchMedia
    try {
      const { wrapper } = await openApp('/hoje')
      await menuButton(wrapper).trigger('click')
      expect(menuButton(wrapper).attributes('aria-expanded')).toBe('true')

      for (const listener of listeners) listener({ matches: false })
      await flushPromises()

      expect(menuButton(wrapper).attributes('aria-expanded')).toBe('false')
      expect(wrapper.get('main').attributes('inert')).toBeUndefined()
    } finally {
      // @ts-expect-error jsdom has no matchMedia; remove the stub so other tests see none.
      delete window.matchMedia
    }
  })
})
