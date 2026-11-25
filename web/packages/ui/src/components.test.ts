import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import BaseButton from './components/BaseButton.vue'
import StatusBadge from './components/StatusBadge.vue'
import { isPhase, phases, toneForPhase } from './phase'

describe('toneForPhase', () => {
  it('gives every phase a tone', () => {
    for (const phase of phases) expect(toneForPhase(phase)).toBeTruthy()
  })

  it.each([
    ['awaiting_deposit', 'planned'],
    ['sourcing', 'waiting'],
    ['assembling', 'assembly'],
    ['ready', 'ready'],
    ['handed_over', 'delivered'],
    ['cancelled', 'neutral'],
  ] as const)('maps %s to %s', (phase, tone) => {
    expect(toneForPhase(phase)).toBe(tone)
  })

  it('recognizes only known phases', () => {
    expect(isPhase('sourcing')).toBe(true)
    expect(isPhase('Compras')).toBe(false)
  })
})

describe('StatusBadge', () => {
  // Two stores, two workflows: the same phase gets the same color whatever its label.
  it.each([
    ['Itens chegando', 'sourcing'],
    ['Na fila', 'sourcing'],
  ] as const)('shows "%s" with the %s color', (label, phase) => {
    const wrapper = mount(StatusBadge, { props: { label, phase } })

    expect(wrapper.text()).toBe(label)
    expect(wrapper.attributes('data-tone')).toBe('waiting')
    expect(wrapper.find('svg').exists()).toBe(true) // icon + word, never color alone
  })
})

describe('BaseButton', () => {
  it('renders the variant and emits click', async () => {
    const wrapper = mount(BaseButton, { props: { variant: 'primary' }, slots: { default: 'Salvar' } })

    await wrapper.trigger('click')

    expect(wrapper.classes()).toContain('enc-button--primary')
    expect(wrapper.attributes('type')).toBe('button')
    expect(wrapper.emitted('click')).toHaveLength(1)
  })

  it('does not emit when disabled', async () => {
    const wrapper = mount(BaseButton, { props: { disabled: true }, slots: { default: 'Salvar' } })

    await wrapper.trigger('click')

    expect(wrapper.emitted('click')).toBeUndefined()
  })
})
