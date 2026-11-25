import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it } from 'vitest'
import { defineComponent, h } from 'vue'

import { useSessionStore } from '../stores/session'
import { authSession } from '../testing/fixtures'
import { usePermission } from './usePermission'

beforeEach(() => setActivePinia(createPinia()))

/** A screen with one action that only members with order:write may use. */
const OrderActions = defineComponent({
  setup() {
    const canWrite = usePermission('order:write')
    return () => h('div', [h('h2', 'Pedido P-0231'), canWrite.value ? h('button', 'Cancelar pedido') : null])
  },
})

describe('usePermission', () => {
  it('hides the action from a signed-out user', () => {
    const wrapper = mount(OrderActions)

    expect(wrapper.find('button').exists()).toBe(false)
  })

  it('hides the action when the role lacks the permission', () => {
    useSessionStore().start(authSession({ permissions: ['order:read'] }))

    const wrapper = mount(OrderActions)

    expect(wrapper.find('button').exists()).toBe(false)
  })

  it('shows the action when the session holds the permission', () => {
    useSessionStore().start(authSession({ permissions: ['order:read', 'order:write'] }))

    const wrapper = mount(OrderActions)

    expect(wrapper.get('button').text()).toBe('Cancelar pedido')
  })

  it('follows the session as it changes', async () => {
    const session = useSessionStore()
    const wrapper = mount(OrderActions)

    session.start(authSession({ permissions: ['order:write'] }))
    await wrapper.vm.$nextTick()
    expect(wrapper.find('button').exists()).toBe(true)

    session.end('logged_out')
    await wrapper.vm.$nextTick()
    expect(wrapper.find('button').exists()).toBe(false)
  })
})
