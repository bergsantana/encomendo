import { enableAutoUnmount, flushPromises } from '@vue/test-utils'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'

import { button, closeApps, field, link, openApp, submitForm } from '../../testing/app'
import { problem } from '../../testing/fixtures'

vi.mock('../../pageLoad', () => ({ loadPage: vi.fn() }))

const server = setupServer()
beforeAll(() => server.listen({ onUnhandledFrame: 'error' }))
afterEach(() => server.resetHandlers())
afterAll(() => server.close())
enableAutoUnmount(afterEach)
afterEach(closeApps)

beforeEach(() => server.use(http.post('*/auth/refresh', () => problem(401, 'unauthenticated', 'Sessão expirada'))))

/** The reset request, answered 202 whether or not the account exists, counting the calls. */
function codesRequested() {
  const emails: unknown[] = []
  server.use(
    http.post('*/auth/password-reset', async ({ request }) => {
      emails.push(await request.json())
      return new HttpResponse(null, { status: 202 })
    }),
  )
  return emails
}

/** openAtCodeStep asks for a code for Ana, so the form shows the code step. */
async function openAtCodeStep() {
  const emails = codesRequested()
  const app = await openApp('/esqueci-a-senha')
  await submitForm(app.wrapper, { 'E-mail': ' ana@altarpm.com.br ' })
  return { ...app, emails }
}

describe('forgot password', () => {
  it('is linked from login', async () => {
    const { wrapper } = await openApp('/entrar')

    expect(link(wrapper, 'Esqueci a senha').attributes('href')).toBe('/esqueci-a-senha')
  })

  it('asks for the email, then for the code, without saying whether the account exists', async () => {
    const { wrapper, emails } = await openAtCodeStep()

    expect(emails).toEqual([{ email: 'ana@altarpm.com.br' }])
    expect(wrapper.get('[role=status]').text()).toBe(
      'Se houver uma conta com ana@altarpm.com.br, enviamos um código de 6 dígitos. Ele vale por 10 minutos.',
    )
    expect(field(wrapper, 'Código').attributes()).toMatchObject({ autocomplete: 'one-time-code', inputmode: 'numeric' })
    expect(field(wrapper, 'Nova senha').attributes('autocomplete')).toBe('new-password')
  })

  it('sets the new password and returns to login with a notice', async () => {
    let body: unknown
    server.use(
      http.post('*/auth/password-reset/confirm', async ({ request }) => {
        body = await request.json()
        return new HttpResponse(null, { status: 204 })
      }),
    )
    const { wrapper, router } = await openAtCodeStep()

    await submitForm(wrapper, { Código: ' 482913 ', 'Nova senha': 'outra-senha-forte' })

    expect(body).toEqual({ email: 'ana@altarpm.com.br', code: '482913', password: 'outra-senha-forte' })
    await vi.waitFor(() => expect(router.currentRoute.value.name).toBe('login'))
    await flushPromises()
    expect(wrapper.get('[role=status]').text()).toBe('Senha alterada. Entre com a nova senha.')
  })

  it('accepts a code pasted with spaces', async () => {
    let body: unknown
    server.use(
      http.post('*/auth/password-reset/confirm', async ({ request }) => {
        body = await request.json()
        return new HttpResponse(null, { status: 204 })
      }),
    )
    const { wrapper } = await openAtCodeStep()

    await submitForm(wrapper, { Código: '482 913', 'Nova senha': 'outra-senha-forte' })

    expect(body).toMatchObject({ code: '482913' })
  })

  it('tells the other tabs, since every login of the account has ended', async () => {
    const heard: unknown[] = []
    const listener = new BroadcastChannel('encomendo-session')
    listener.onmessage = (event: MessageEvent) => heard.push(event.data)
    server.use(http.post('*/auth/password-reset/confirm', () => new HttpResponse(null, { status: 204 })))
    const { wrapper } = await openAtCodeStep()

    await submitForm(wrapper, { Código: '482913', 'Nova senha': 'outra-senha-forte' })

    await vi.waitFor(() => expect(heard).toEqual([{ userId: null, tenantId: null }]))
    listener.close()
  })

  it('stops showing the notice once the member moves on from login', async () => {
    server.use(http.post('*/auth/password-reset/confirm', () => new HttpResponse(null, { status: 204 })))
    const { wrapper, router } = await openAtCodeStep()
    await submitForm(wrapper, { Código: '482913', 'Nova senha': 'outra-senha-forte' })
    await vi.waitFor(() => expect(router.currentRoute.value.name).toBe('login'))

    await router.push('/criar-conta')
    await router.push('/entrar')
    await flushPromises()

    expect(wrapper.find('[role=status]').exists()).toBe(false)
  })

  it('checks that the code has 6 digits before calling the API', async () => {
    let calls = 0
    server.use(
      http.post('*/auth/password-reset/confirm', () => {
        calls += 1
        return new HttpResponse(null, { status: 204 })
      }),
    )
    const { wrapper } = await openAtCodeStep()

    await submitForm(wrapper, { Código: '48291a', 'Nova senha': 'outra-senha-forte' })

    expect(calls).toBe(0)
    expect(wrapper.text()).toContain('Informe os 6 dígitos do código.')
    expect(document.activeElement).toBe(field(wrapper, 'Código').element)
  })

  it('says so next to the code when it is wrong or expired', async () => {
    server.use(http.post('*/auth/password-reset/confirm', () => problem(400, 'invalid_code', 'Código inválido')))
    const { wrapper } = await openAtCodeStep()

    await submitForm(wrapper, { Código: '000000', 'Nova senha': 'outra-senha-forte' })

    const code = field(wrapper, 'Código')
    expect(code.attributes('aria-invalid')).toBe('true')
    expect(wrapper.text()).toContain('Código errado ou vencido. Confira o e-mail ou peça um novo código.')
  })

  it('sends another code', async () => {
    const { wrapper, emails } = await openAtCodeStep()

    await button(wrapper, 'Enviar outro código').trigger('click')
    await flushPromises()

    expect(emails).toHaveLength(2)
    expect(wrapper.get('[role=status]').text()).toBe('Se houver uma conta com ana@altarpm.com.br, enviamos um novo código. O anterior deixou de valer.')
  })

  it('lets the member start over with another email', async () => {
    const { wrapper } = await openAtCodeStep()

    await button(wrapper, 'Usar outro e-mail').trigger('click')
    await flushPromises()

    expect(wrapper.findAll('label').map((l) => l.text())).toEqual(['E-mail'])
    // The button that was clicked is gone; focus lands on the field to fill.
    expect(document.activeElement).toBe(field(wrapper, 'E-mail').element)
  })

  it('tells how long to wait after too many requests', async () => {
    server.use(http.post('*/auth/password-reset', () => problem(429, 'rate_limited', 'Muitas tentativas', { 'Retry-After': '600' })))
    const { wrapper } = await openApp('/esqueci-a-senha')

    await submitForm(wrapper, { 'E-mail': 'ana@altarpm.com.br' })

    expect(wrapper.get('[role=alert]').text()).toBe('Muitas tentativas. Tente de novo em 10 minutos.')
  })
})
