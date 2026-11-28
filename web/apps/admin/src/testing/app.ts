import { type DOMWrapper, flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory } from 'vue-router'

import App from '../App.vue'
import { createAppRouter } from '../router'
import { useSessionStore } from '../stores/session'

const opened: ReturnType<typeof createPinia>[] = []

/**
 * closeApps disposes the session store of every app opened so far, closing
 * its BroadcastChannel, so no announcement from one test reaches the next.
 * Call it after each test, next to enableAutoUnmount.
 */
export function closeApps(): void {
  for (const pinia of opened.splice(0)) useSessionStore(pinia).$dispose()
}

/** openApp loads the admin app at a path, as a reload or a pasted link does. */
export async function openApp(path: string) {
  const pinia = createPinia()
  opened.push(pinia)
  setActivePinia(pinia)
  const router = createAppRouter(createMemoryHistory())
  await router.push(path)
  // Attached, so focus moves can be checked.
  const wrapper = mount(App, { global: { plugins: [pinia, router] }, attachTo: document.body })
  await flushPromises()
  return { wrapper, router }
}

/** Scope is where a helper looks: the whole app, or one part of it such as a table row. */
type Scope = VueWrapper | DOMWrapper<Element>

/** field finds an input by its visible label, as a user does: a label for it, or the label that wraps it (a checkbox or radio). */
export function field(wrapper: Scope, label: string) {
  const found = wrapper.findAll('label').find((l) => l.text() === label)
  if (found === undefined) throw new Error(`no field labelled ${label}`)
  const id = found.attributes('for')
  return id === undefined ? found.get('input') : wrapper.get(`#${id}`)
}

/** button finds a button by its visible text. */
export function button(wrapper: Scope, name: string) {
  const found = wrapper.findAll('button').find((b) => b.text() === name)
  if (found === undefined) throw new Error(`no button ${name}`)
  return found
}

/** link finds a link by its visible text. */
export function link(wrapper: Scope, name: string) {
  const found = wrapper.findAll('a').find((a) => a.text() === name)
  if (found === undefined) throw new Error(`no link ${name}`)
  return found
}

/** fill types into fields by label, then submits the form. */
export async function submitForm(wrapper: VueWrapper, values: Record<string, string>) {
  for (const [label, value] of Object.entries(values)) await field(wrapper, label).setValue(value)
  await wrapper.get('form').trigger('submit')
  await flushPromises()
}
