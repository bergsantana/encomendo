import { enableAutoUnmount, flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { createPinia, setActivePinia } from 'pinia'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory } from 'vue-router'

import { api } from '../../api'
import App from '../../App.vue'
import { loadPage } from '../../pageLoad'
import { createAppRouter } from '../../router'
import { useSessionStore } from '../../stores/session'
import { authSession, gate, problem } from '../../testing/fixtures'

// A full page load cannot happen in jsdom; the tests check where it would go.
vi.mock('../../pageLoad', () => ({ loadPage: vi.fn() }))

const server = setupServer()
beforeAll(() => server.listen({ onUnhandledFrame: 'error' }))
afterEach(() => {
  server.resetHandlers()
  vi.mocked(loadPage).mockClear()
})
afterAll(() => server.close())
enableAutoUnmount(afterEach)

const ready = http.get('*/readyz', () => HttpResponse.json({ status: 'ok' }))
const noSession = http.post('*/auth/refresh', () => problem(401, 'unauthenticated', 'Sessão expirada'))
const signedIn = http.post('*/auth/refresh', () => HttpResponse.json(authSession()))

beforeEach(() => server.use(ready, noSession))

/** openApp loads the admin app at a path, as a reload or a pasted link does. */
async function openApp(path: string) {
  const pinia = createPinia()
  setActivePinia(pinia)
  const router = createAppRouter(createMemoryHistory())
  await router.push(path)
  // Attached, so focus moves can be checked.
  const wrapper = mount(App, { global: { plugins: [pinia, router] }, attachTo: document.body })
  await flushPromises()
  return { wrapper, router }
}

/** field finds an input by its visible label, as a user does. */
function field(wrapper: VueWrapper, label: string) {
  const id = wrapper.findAll('label').find((l) => l.text() === label)?.attributes('for')
  if (id === undefined) throw new Error(`no field labelled ${label}`)
  return wrapper.get(`#${id}`)
}

function button(wrapper: VueWrapper, name: string) {
  const found = wrapper.findAll('button').find((b) => b.text() === name)
  if (found === undefined) throw new Error(`no button ${name}`)
  return found
}

async function logIn(wrapper: VueWrapper, email: string, password: string) {
  await field(wrapper, 'E-mail').setValue(email)
  await field(wrapper, 'Senha').setValue(password)
  await wrapper.get('form').trigger('submit')
  await flushPromises()
}

const loginAnswers = (answer: () => Response) => server.use(http.post('*/auth/login', answer))

describe('login', () => {
  it('opens on the login form when signed out', async () => {
    const { wrapper, router } = await openApp('/')

    expect(router.currentRoute.value.name).toBe('login')
    expect(wrapper.get('h1').text()).toBe('Entrar')
    expect(field(wrapper, 'E-mail').attributes()).toMatchObject({ autocomplete: 'username', 'aria-required': 'true' })
    expect(field(wrapper, 'Senha').attributes()).toMatchObject({ type: 'password', 'aria-required': 'true' })
    expect(button(wrapper, 'Entrar').attributes('type')).toBe('submit')
    expect(wrapper.find('[role=alert]').exists()).toBe(false)
  })

  it('asks for the missing fields without calling the API, and moves to the first one', async () => {
    let calls = 0
    loginAnswers(() => {
      calls += 1
      return HttpResponse.json(authSession())
    })
    const { wrapper } = await openApp('/entrar')

    await logIn(wrapper, '  ', '')

    expect(calls).toBe(0)
    expect(wrapper.get('[role=alert]').text()).toBe('Confira os campos destacados.')
    expect(wrapper.text()).toContain('Informe seu e-mail.')
    expect(wrapper.text()).toContain('Informe sua senha.')
    expect(document.activeElement).toBe(field(wrapper, 'E-mail').element)
  })

  it('logs in and opens the page the member was going to', async () => {
    let body: unknown
    server.use(
      http.post('*/auth/login', async ({ request }) => {
        body = await request.json()
        return HttpResponse.json(authSession())
      }),
    )
    const { wrapper, router } = await openApp('/entrar?redirect=/hoje')

    await logIn(wrapper, ' ana@altarpm.com.br ', 'segredo-forte')

    expect(body).toEqual({ email: 'ana@altarpm.com.br', password: 'segredo-forte' })
    // The next screen is lazy-loaded, so the navigation settles a few ticks later.
    await vi.waitFor(() => expect(router.currentRoute.value.name).toBe('hoje'))
    await flushPromises()
    expect(wrapper.get('h1').text()).toBe('Hoje')
  })

  it('opens Hoje when the redirect points outside the app', async () => {
    loginAnswers(() => HttpResponse.json(authSession()))
    const { wrapper, router } = await openApp('/entrar?redirect=//evil.example')

    await logIn(wrapper, 'ana@altarpm.com.br', 'segredo-forte')

    await vi.waitFor(() => expect(router.currentRoute.value.name).toBe('hoje'))
  })

  it.each([
    ['a wrong email or password', () => problem(401, 'invalid_credentials', 'Credenciais inválidas'), 'E-mail ou senha incorretos.'],
    [
      'too many attempts',
      () => problem(429, 'rate_limited', 'Muitas tentativas', { 'Retry-After': '90' }),
      'Muitas tentativas. Tente de novo em 2 minutos.',
    ],
    ['no connection', () => HttpResponse.error(), 'Sem conexão com o servidor. Confira a internet e tente de novo.'],
    ['a server failure', () => problem(503, 'not_ready', 'Serviço indisponível'), 'O servidor não respondeu. Tente de novo em instantes.'],
  ])('says so on %s, and stays on the form', async (_, answer, message) => {
    loginAnswers(answer)
    const { wrapper, router } = await openApp('/entrar')

    await logIn(wrapper, 'ana@altarpm.com.br', 'segredo-forte')

    expect(wrapper.get('[role=alert]').text()).toBe(message)
    expect(router.currentRoute.value.name).toBe('login')
  })

  it("shows the API's field messages next to the fields", async () => {
    loginAnswers(
      () =>
        new HttpResponse(
          JSON.stringify({
            type: 'about:blank',
            title: 'Dados inválidos',
            status: 400,
            code: 'invalid',
            errors: [{ field: 'email', message: 'Informe um e-mail válido.' }],
          }),
          { status: 400, headers: { 'Content-Type': 'application/problem+json' } },
        ),
    )
    const { wrapper } = await openApp('/entrar')

    await logIn(wrapper, 'ana', 'segredo-forte')

    const email = field(wrapper, 'E-mail')
    expect(wrapper.get(`#${email.attributes('aria-describedby')}`).text()).toBe('Informe um e-mail válido.')
    expect(wrapper.get('[role=alert]').text()).toBe('Confira os campos destacados.')
    expect(document.activeElement).toBe(email.element)
  })

  it('says why when the session could not be restored on load', async () => {
    // The cookie may still be good; the member should know the server is the problem.
    server.use(http.post('*/auth/refresh', () => problem(503, 'not_ready', 'Serviço indisponível')))

    const { wrapper } = await openApp('/hoje')

    expect(wrapper.get('h1').text()).toBe('Entrar')
    expect(wrapper.get('[role=alert]').text()).toBe('O servidor não respondeu. Tente de novo em instantes.')
  })
})

describe('the session in the menu', () => {
  it('shows who is signed in, in which store', async () => {
    server.use(signedIn)

    const { wrapper } = await openApp('/')

    const menu = wrapper.get('nav')
    expect(menu.text()).toContain('Ana Souza')
    expect(menu.text()).toContain('Alta RPM')
  })

  it('logs out with a full page load to login, so nothing of the session stays in memory', async () => {
    let authorization: string | null = null
    server.use(
      signedIn,
      http.post('*/auth/logout', ({ request }) => {
        authorization = request.headers.get('Authorization')
        return new HttpResponse(null, { status: 204 })
      }),
    )
    const { wrapper } = await openApp('/hoje?mes=2026-11')

    await button(wrapper, 'Sair').trigger('click')
    await flushPromises()

    expect(authorization).toBe('Bearer access-1')
    // No redirect: the next person to log in should not land on this member's page.
    expect(loadPage).toHaveBeenCalledWith('/entrar')
  })

  it('disables Sair while logging out, so a double click sends one logout', async () => {
    const answer = gate()
    server.use(
      signedIn,
      http.post('*/auth/logout', async () => {
        await answer.opened
        return new HttpResponse(null, { status: 204 })
      }),
    )
    const { wrapper } = await openApp('/')

    await button(wrapper, 'Sair').trigger('click')

    expect(button(wrapper, 'Sair').attributes('disabled')).toBeDefined()
    answer.open()
    await flushPromises()
  })

  it('stays signed in and says so when the server cannot be reached', async () => {
    server.use(
      signedIn,
      http.post('*/auth/logout', () => HttpResponse.error()),
    )
    const { wrapper, router } = await openApp('/')

    await button(wrapper, 'Sair').trigger('click')
    await flushPromises()

    expect(router.currentRoute.value.name).toBe('hoje')
    expect(loadPage).not.toHaveBeenCalled()
    expect(wrapper.get('[role=alert]').text()).toBe('Não foi possível sair. Sem conexão com o servidor. Confira a internet e tente de novo.')
  })

  it('sends the member to login, and back here afterwards, when the session expires while working', async () => {
    server.use(
      // The reload restores the session; the next refresh is refused.
      http.post('*/auth/refresh', () => HttpResponse.json(authSession()), { once: true }),
      http.post('*/auth/refresh', () => problem(401, 'unauthenticated', 'Sessão expirada')),
      http.get('*/api/v1/admin/members', () => problem(401, 'unauthenticated', 'Sessão expirada')),
    )
    await openApp('/hoje?mes=2026-11#topo')

    // Any call a screen makes with the expired token.
    await expect(api.get('/api/v1/admin/members')).rejects.toMatchObject({ status: 401 })
    await flushPromises()

    expect(loadPage).toHaveBeenCalledWith('/entrar?redirect=/hoje?mes=2026-11')
  })

  it('sends the member to login when another tab logged in or switched store', async () => {
    server.use(signedIn)
    await openApp('/')

    useSessionStore().end('replaced')
    await flushPromises()

    expect(loadPage).toHaveBeenCalledWith('/entrar')
  })
})
