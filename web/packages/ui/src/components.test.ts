import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import { defineComponent, h } from 'vue'

import BaseButton from './components/BaseButton.vue'
import BaseDialog from './components/BaseDialog.vue'
import BaseInput from './components/BaseInput.vue'
import BaseSelect from './components/BaseSelect.vue'
import StatusBadge from './components/StatusBadge.vue'
import SuspendedBadge from './components/SuspendedBadge.vue'
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

describe('BaseSelect', () => {
  const roles = [
    { value: 'admin', label: 'Admin' },
    { value: 'staff', label: 'Funcionário' },
  ]
  const select = (props: Record<string, unknown> = {}) =>
    mount(BaseSelect, { props: { label: 'Papel', modelValue: 'staff', options: roles, ...props } })

  it('labels the choice and shows every option, with the current one selected', () => {
    const wrapper = select()
    const element = wrapper.get('select')

    expect(wrapper.get(`label[for="${element.attributes('id')}"]`).text()).toBe('Papel')
    expect(wrapper.findAll('option').map((o) => o.text())).toEqual(['Admin', 'Funcionário'])
    expect((element.element as HTMLSelectElement).value).toBe('staff')
  })

  it('emits the value the user picks', async () => {
    const wrapper = select()

    await wrapper.get('select').setValue('admin')

    expect(wrapper.emitted('update:modelValue')).toEqual([['admin']])
  })

  it('shows the error next to the choice and links it', () => {
    const wrapper = select({ error: 'Escolha um papel.' })
    const element = wrapper.get('select')

    expect(element.attributes('aria-invalid')).toBe('true')
    expect(wrapper.get(`#${element.attributes('aria-describedby')}`).text()).toBe('Escolha um papel.')
  })

  it('can be turned off while a change is saved', () => {
    expect(select({ disabled: true }).get('select').attributes('disabled')).toBeDefined()
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

  it('shows a hint under the field and links it for screen readers', () => {
    const wrapper = field({ hint: 'Pelo menos 8 caracteres.' })
    const describedBy = wrapper.get('input').attributes('aria-describedby') ?? ''

    expect(wrapper.get(`#${describedBy}`).text()).toBe('Pelo menos 8 caracteres.')
  })

  it('links both the hint and the error when there are both', () => {
    const wrapper = field({ hint: 'Pelo menos 8 caracteres.', error: 'Informe uma senha.' })
    const ids = (wrapper.get('input').attributes('aria-describedby') ?? '').split(' ')

    expect(ids.map((id) => wrapper.get(`#${id}`).text())).toEqual(['Informe uma senha.', 'Pelo menos 8 caracteres.'])
  })

  it('passes other attributes on to the input, such as a numeric keyboard', () => {
    const wrapper = mount(BaseInput, { props: { label: 'Código', modelValue: '' }, attrs: { inputmode: 'numeric', maxlength: '6' } })

    expect(wrapper.get('input').attributes()).toMatchObject({ inputmode: 'numeric', maxlength: '6' })
    expect(wrapper.attributes('inputmode')).toBeUndefined()
  })

  it('keeps its own id and links even when a parent passes others', () => {
    const wrapper = mount(BaseInput, { props: { label: 'E-mail', modelValue: '', error: 'Informe seu e-mail.' }, attrs: { id: 'x', 'aria-describedby': 'y' } })
    const input = wrapper.get('input')

    expect(wrapper.get('label').attributes('for')).toBe(input.attributes('id'))
    expect(input.attributes('id')).not.toBe('x')
    expect(wrapper.get(`#${input.attributes('aria-describedby')}`).text()).toBe('Informe seu e-mail.')
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

describe('SuspendedBadge', () => {
  it('says "Suspensa" with an icon hidden from screen readers, so it is never color alone', () => {
    const wrapper = mount(SuspendedBadge, { attrs: { id: 'suspended-1' } })

    expect(wrapper.text()).toBe('Suspensa')
    expect(wrapper.attributes('id')).toBe('suspended-1')
    expect(wrapper.get('svg').attributes('aria-hidden')).toBe('true')
  })
})

describe('BaseDialog', () => {
  const dialog = (variant?: 'dialog' | 'drawer') =>
    mount(BaseDialog, {
      props: { title: 'Editar item', variant },
      slots: { default: '<label for="x">Nome</label><input id="x"><button type="button">Salvar</button>' },
      attachTo: document.body,
    })

  it('is named by its title and takes focus when it opens', async () => {
    const wrapper = dialog()
    await wrapper.vm.$nextTick()
    await wrapper.vm.$nextTick()

    const surface = wrapper.get('[role=dialog]')
    expect(surface.attributes('aria-modal')).toBe('true')
    expect(document.getElementById(surface.attributes('aria-labelledby') ?? '')?.textContent).toBe('Editar item')
    expect(document.activeElement).toBe(surface.element)
    wrapper.unmount()
  })

  it('asks to close on Escape and on the backdrop', async () => {
    const wrapper = dialog('drawer')

    await wrapper.get('[role=dialog]').trigger('keydown', { key: 'Escape' })
    await wrapper.get('.enc-dialog__backdrop').trigger('click')

    expect(wrapper.emitted('close')).toHaveLength(2)
    wrapper.unmount()
  })

  it('keeps Tab inside, in both directions', async () => {
    const wrapper = dialog()
    const surface = wrapper.get('[role=dialog]')
    const input = wrapper.get('input').element
    const button = wrapper.get('button').element
    await wrapper.vm.$nextTick()
    await wrapper.vm.$nextTick()

    button.focus()
    await surface.trigger('keydown', { key: 'Tab' })
    expect(document.activeElement).toBe(input)

    await surface.trigger('keydown', { key: 'Tab', shiftKey: true })
    expect(document.activeElement).toBe(button)
    wrapper.unmount()
  })

  it('hands focus back to what opened it', async () => {
    const opener = document.createElement('button')
    document.body.append(opener)
    opener.focus()
    const wrapper = dialog()
    await wrapper.vm.$nextTick()

    wrapper.unmount()

    expect(document.activeElement).toBe(opener)
    opener.remove()
  })
})
