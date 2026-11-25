import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import App from './App.vue'
import { resolveSite } from './site'

describe('resolveSite', () => {
  it.each([
    ['altarpm.encomendo.com.br', { kind: 'store', slug: 'altarpm' }],
    ['AltaRPM.Encomendo.com.br:443', { kind: 'store', slug: 'altarpm' }],
    ['marcenaria-silva.encomendo.com.br', { kind: 'store', slug: 'marcenaria-silva' }],
    ['conta.encomendo.com.br', { kind: 'account' }],
    ['admin.encomendo.com.br', { kind: 'unknown' }],
    ['www.encomendo.com.br', { kind: 'unknown' }],
    ['encomendo.com.br', { kind: 'unknown' }],
    ['a.b.encomendo.com.br', { kind: 'unknown' }],
    ['-bad.encomendo.com.br', { kind: 'unknown' }],
    ['altarpm.example.com', { kind: 'unknown' }],
    ['encomendo.com.br.evil.com', { kind: 'unknown' }],
  ])('%s', (host, want) => {
    expect(resolveSite(host, 'encomendo.com.br')).toEqual(want)
  })

  it('works with the local base domain', () => {
    expect(resolveSite('altarpm.localhost:5174', 'localhost')).toEqual({ kind: 'store', slug: 'altarpm' })
  })
})

describe('App', () => {
  it('shows the store page for a store', () => {
    const wrapper = mount(App, { props: { site: { kind: 'store', slug: 'altarpm' } } })
    expect(wrapper.get('h1').text()).toBe('Loja altarpm')
  })

  it('shows the client area on conta', () => {
    const wrapper = mount(App, { props: { site: { kind: 'account' } } })
    expect(wrapper.get('h1').text()).toBe('Minhas encomendas')
  })

  it('shows not found for anything else', () => {
    const wrapper = mount(App, { props: { site: { kind: 'unknown' } } })
    expect(wrapper.get('h1').text()).toBe('Loja não encontrada')
  })
})
