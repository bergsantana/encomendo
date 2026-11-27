import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import { defineComponent, h } from 'vue'

import { phoneMaxWidth, phoneQuery, useIsPhone, useMediaQuery } from './responsive'

/** fakeMedia installs a matchMedia whose answer a test can change; it returns that and a cleanup. */
function fakeMedia(initial: boolean) {
  const listeners = new Set<(event: MediaQueryListEvent) => void>()
  const queries: string[] = []
  window.matchMedia = ((query: string) => {
    queries.push(query)
    return {
      matches: initial,
      media: query,
      addEventListener: (_: string, listener: (event: MediaQueryListEvent) => void) => listeners.add(listener),
      removeEventListener: (_: string, listener: (event: MediaQueryListEvent) => void) => listeners.delete(listener),
    }
  }) as unknown as typeof window.matchMedia
  return {
    listeners,
    queries,
    change: (matches: boolean) => listeners.forEach((listener) => listener({ matches } as MediaQueryListEvent)),
    restore: () => {
      // @ts-expect-error jsdom has no matchMedia; remove the stub so other tests see none.
      delete window.matchMedia
    },
  }
}

const probe = (use: () => { value: boolean }) =>
  mount(
    defineComponent({
      setup() {
        const matches = use()
        return () => h('span', String(matches.value))
      },
    }),
  )

describe('useMediaQuery', () => {
  it('is false where the browser has no matchMedia', () => {
    expect(probe(() => useMediaQuery('(max-width: 1px)')).text()).toBe('false')
  })

  it('starts from the current answer, follows changes and stops listening on unmount', async () => {
    const media = fakeMedia(true)
    try {
      const wrapper = probe(() => useMediaQuery('(max-width: 1px)'))
      expect(wrapper.text()).toBe('true')

      media.change(false)
      await wrapper.vm.$nextTick()
      expect(wrapper.text()).toBe('false')

      wrapper.unmount()
      expect(media.listeners.size).toBe(0)
    } finally {
      media.restore()
    }
  })
})

describe('useIsPhone', () => {
  it('asks for 640 px and narrower, the breakpoint tokens.css uses', () => {
    const media = fakeMedia(true)
    try {
      expect(probe(useIsPhone).text()).toBe('true')
      expect(media.queries).toEqual([phoneQuery])
      expect(phoneMaxWidth).toBe(640)
    } finally {
      media.restore()
    }
  })
})
