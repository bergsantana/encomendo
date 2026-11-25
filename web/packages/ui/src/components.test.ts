import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import { defineComponent, h } from 'vue'

import BaseButton from './components/BaseButton.vue'
import BaseInput from './components/BaseInput.vue'
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

describe('BaseInput', () => {
  const field = (props: Record<string, unknown> = {}) =>
    mount(BaseInput, { props: { label: 'E-mail', modelValue: '', type: 'email', autocomplete: 'username', ...props } })

  it('labels the field so it can be found by its name', () => {
    const wrapper = field()
    const input = wrapper.get('input')

    expect(wrapper.get(`label[for="${input.attributes('id')}"]`).text()).toBe('E-mail')
    expect(input.attributes()).toMatchObject({ type: 'email', autocomplete: 'username' })
    expect(input.attributes('aria-invalid')).toBeUndefined()
  })

  it('emits what the user types', async () => {
    const wrapper = field()

    await wrapper.get('input').setValue('ana@altarpm.com.br')

    expect(wrapper.emitted('update:modelValue')).toEqual([['ana@altarpm.com.br']])
  })

  it('shows the error next to the field and links it for screen readers', () => {
    const wrapper = field({ error: 'Informe seu e-mail.' })
    const input = wrapper.get('input')
    const describedBy = input.attributes('aria-describedby')

    expect(input.attributes('aria-invalid')).toBe('true')
    expect(describedBy).toBeTruthy()
    expect(wrapper.get(`#${describedBy}`).text()).toBe('Informe seu e-mail.')
    expect(wrapper.find('svg').exists()).toBe(true) // an icon, so the error is not color alone
  })

  it('marks a required field for the browser and for screen readers', () => {
    const input = field({ required: true }).get('input')

    expect(input.attributes('required')).toBeDefined()
    expect(input.attributes('aria-required')).toBe('true')
  })

  it('lets the form move focus to it, e.g. to the first field to fix', () => {
    const wrapper = mount(BaseInput, { props: { label: 'E-mail', modelValue: '' }, attachTo: document.body })

    ;(wrapper.vm as unknown as { focus: () => void }).focus()

    expect(document.activeElement).toBe(wrapper.get('input').element)
    wrapper.unmount()
  })

  it('gives each field in a form its own id', () => {
    const form = defineComponent({
      render: () =>
        h('form', [
          h(BaseInput, { label: 'E-mail', modelValue: '' }),
          h(BaseInput, { label: 'Senha', modelValue: '', type: 'password' }),
        ]),
    })

    const [email, password] = mount(form).findAll('input')

    expect(email?.attributes('id')).toBeTruthy()
    expect(email?.attributes('id')).not.toBe(password?.attributes('id'))
  })
})
