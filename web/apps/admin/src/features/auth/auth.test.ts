import { enableAutoUnmount, flushPromises, type VueWrapper } from '@vue/test-utils'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '../../api'
import { rememberStore } from '../../lastStore'
import { loadPage } from '../../pageLoad'
import { useSessionStore } from '../../stores/session'
import { button, closeApps, field, link, openApp, submitForm } from '../../testing/app'
import { altaRpm, authSession, gate, problem } from '../../testing/fixtures'

// A full page load cannot happen in jsdom; the tests check where it would go.
vi.mock('../../pageLoad', () => ({ loadPage: vi.fn() }))

const server = setupServer()
beforeAll(() => server.listen({ onUnhandledFrame: 'error' }))
afterEach(() => {
  server.resetHandlers()
  vi.mocked(loadPage).mockClear()
  localStorage.clear()
})
afterAll(() => server.close())
enableAutoUnmount(afterEach)
afterEach(closeApps)

const ready = http.get('*/readyz', () => HttpResponse.json({ status: 'ok' }))
const noSession = http.post('*/auth/refresh', () => problem(401, 'unauthenticated', 'Sessão expirada'))
const signedIn = http.post('*/auth/refresh', () => HttpResponse.json(authSession()))

beforeEach(() => server.use(ready, noSession))

const logIn = (wrapper: VueWrapper, email: string, password: string) => submitForm(wrapper, { 'E-mail': email, Senha: password })

const loginAnswers = (answer: () => Response) => server.use(http.post('*/auth/login', answer))

describe('login', () => {
  it('offers to create an account', async () => {
    const { wrapper } = await openApp('/entrar')

    expect(link(wrapper, 'Criar conta').attributes('href')).toBe('/criar-conta')
  })

  it('opens the store this device used last', async () => {
    rememberStore(altaRpm.id)
    let body: unknown
    server.use(
      http.post('*/auth/login', async ({ request }) => {
        body = await request.json()
        return HttpResponse.json(authSession())
      }),
    )
    const { wrapper } = await openApp('/entrar')

    await logIn(wrapper, 'ana@altarpm.com.br', 'segredo-forte')

    expect(body).toMatchObject({ tenantId: altaRpm.id })
  })

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

  it('tries the session again on request, and opens the page once the server answers', async () => {
    // Typing the password would start a new login; the one in the cookie may still be good.
    server.use(
      http.post('*/auth/refresh', () => problem(503, 'not_ready', 'Serviço indisponível'), { once: true }),
      http.post('*/auth/refresh', () => HttpResponse.json(authSession())),
    )
    const { wrapper, router } = await openApp('/hoje')

    await button(wrapper, 'Tentar de novo').trigger('click')

    await vi.waitFor(() => expect(router.currentRoute.value.name).toBe('hoje'))
  })

  it('keeps saying why when the server still cannot answer, and keeps focus on the button', async () => {
    server.use(http.post('*/auth/refresh', () => problem(503, 'not_ready', 'Serviço indisponível')))
    const { wrapper, router } = await openApp('/hoje')
    const retry = button(wrapper, 'Tentar de novo')
    ;(retry.element as HTMLButtonElement).focus()

    await retry.trigger('click')
    await flushPromises()

    expect(router.currentRoute.value.name).toBe('login')
    expect(wrapper.get('[role=alert]').text()).toBe('O servidor não respondeu. Tente de novo em instantes.')
    expect(document.activeElement).toBe(button(wrapper, 'Tentar de novo').element)
  })

  it('says it is trying again while it waits for the server', async () => {
    const answer = gate()
    server.use(
      http.post('*/auth/refresh', () => problem(503, 'not_ready', 'Serviço indisponível'), { once: true }),
      http.post('*/auth/refresh', async () => {
        await answer.opened
        return problem(503, 'not_ready', 'Serviço indisponível')
      }),
    )
    const { wrapper } = await openApp('/hoje')

    await button(wrapper, 'Tentar de novo').trigger('click')

    expect(wrapper.get('[role=status]').text()).toBe('Tentando de novo…')
    // The message goes while asking, so it is announced again if the server still fails.
    expect(wrapper.find('[role=alert]').exists()).toBe(false)
    answer.open()
    await flushPromises()
    expect(wrapper.find('[role=alert]').exists()).toBe(true)
  })

  it('leaves the login form when the retry finds there is no session after all', async () => {
    server.use(
      http.post('*/auth/refresh', () => problem(503, 'not_ready', 'Serviço indisponível'), { once: true }),
      http.post('*/auth/refresh', () => problem(401, 'unauthenticated', 'Sessão expirada')),
    )
    const { wrapper, router } = await openApp('/hoje')

    await button(wrapper, 'Tentar de novo').trigger('click')
    await flushPromises()

    expect(router.currentRoute.value.name).toBe('login')
    expect(wrapper.find('[role=alert]').exists()).toBe(false)
    expect(wrapper.findAll('button').map((b) => b.text())).not.toContain('Tentar de novo')
  })

  it('offers no retry after too many attempts, since the message asks to wait', async () => {
    server.use(http.post('*/auth/refresh', () => problem(429, 'rate_limited', 'Muitas tentativas', { 'Retry-After': '30' })))

    const { wrapper } = await openApp('/hoje')

    expect(wrapper.get('[role=alert]').text()).toBe('Muitas tentativas. Tente de novo em 30 segundos.')
    expect(wrapper.findAll('button').map((b) => b.text())).not.toContain('Tentar de novo')
  })

  it('offers no retry when there simply is no session', async () => {
    const { wrapper } = await openApp('/entrar')

    expect(wrapper.findAll('button').map((b) => b.text())).not.toContain('Tentar de novo')
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
