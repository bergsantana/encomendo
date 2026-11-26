import type { TenantSummary } from '@encomendo/api'
import { enableAutoUnmount, flushPromises } from '@vue/test-utils'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'

import { loadPage } from '../../pageLoad'
import { button, closeApps, field, link, openApp, submitForm } from '../../testing/app'
import { altaRpm, authSession, problem } from '../../testing/fixtures'

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

const motoPecas: TenantSummary = { id: '0190f3c4-5e1a-7b2c-8d3e-00000000cafe', name: 'Moto Peças Sul', slug: 'motopecas' }

/** A session with no store selected, as after sign-up or a login with several stores. */
const storeless = (memberships = authSession().memberships) =>
  authSession({ tenant: undefined, role: undefined, permissions: [], memberships })

const cookieHolds = (session: ReturnType<typeof authSession> | null) =>
  server.use(
    http.post('*/auth/refresh', () =>
      session === null ? problem(401, 'unauthenticated', 'Sessão expirada') : HttpResponse.json(session),
    ),
  )

beforeEach(() => server.use(http.get('*/readyz', () => HttpResponse.json({ status: 'ok' }))))

describe('sign-up', () => {
  beforeEach(() => cookieHolds(null))

  it('creates the account, then asks for a store', async () => {
    let body: unknown
    server.use(
      http.post('*/auth/signup', async ({ request }) => {
        body = await request.json()
        return HttpResponse.json(storeless([]), { status: 201 })
      }),
    )
    const { wrapper, router } = await openApp('/criar-conta')

    await submitForm(wrapper, { Nome: ' Ana Souza ', 'E-mail': ' ana@altarpm.com.br ', Senha: 'segredo-forte' })

    expect(body).toEqual({ name: 'Ana Souza', email: 'ana@altarpm.com.br', password: 'segredo-forte' })
    await vi.waitFor(() => expect(router.currentRoute.value.name).toBe('stores'))
    await flushPromises()
    expect(wrapper.get('h1').text()).toBe('Crie sua loja')
  })

  it('explains the password rule next to the field', async () => {
    const { wrapper } = await openApp('/criar-conta')

    const password = field(wrapper, 'Senha')
    expect(wrapper.get(`#${password.attributes('aria-describedby')}`).text()).toBe('Pelo menos 8 caracteres.')
    expect(password.attributes('autocomplete')).toBe('new-password')
  })

  it('asks for the missing fields and moves to the first one', async () => {
    const { wrapper } = await openApp('/criar-conta')

    await submitForm(wrapper, { Nome: '', 'E-mail': '', Senha: '' })

    expect(wrapper.get('[role=alert]').text()).toBe('Confira os campos destacados.')
    expect(wrapper.text()).toContain('Informe seu nome.')
    expect(wrapper.text()).toContain('Informe seu e-mail.')
    expect(wrapper.text()).toContain('Informe uma senha.')
    expect(document.activeElement).toBe(field(wrapper, 'Nome').element)
  })

  it('says so next to the email when it already has an account', async () => {
    server.use(http.post('*/auth/signup', () => problem(409, 'email_taken', 'E-mail em uso')))
    const { wrapper } = await openApp('/criar-conta')

    await submitForm(wrapper, { Nome: 'Ana Souza', 'E-mail': 'ana@altarpm.com.br', Senha: 'segredo-forte' })

    const email = field(wrapper, 'E-mail')
    expect(wrapper.get(`#${email.attributes('aria-describedby')}`).text()).toBe('Já existe uma conta com este e-mail.')
    expect(document.activeElement).toBe(email.element)
  })

  it('links back to login', async () => {
    const { wrapper } = await openApp('/criar-conta')

    expect(link(wrapper, 'Já tenho conta').attributes('href')).toBe('/entrar')
  })
})

describe('creating the first store', () => {
  beforeEach(() => cookieHolds(storeless([])))

  it('creates the store, switches to it and opens Hoje', async () => {
    let created: { authorization: string | null; body: unknown } | undefined
    let switchedTo: unknown
    server.use(
      http.post('*/api/v1/admin/tenants', async ({ request }) => {
        created = { authorization: request.headers.get('Authorization'), body: await request.json() }
        return HttpResponse.json({ ...altaRpm, createdAt: '2026-11-25T12:00:00Z' }, { status: 201 })
      }),
      http.post('*/auth/switch-tenant', async ({ request }) => {
        switchedTo = await request.json()
        return HttpResponse.json(authSession())
      }),
    )
    const { wrapper } = await openApp('/lojas')

    await submitForm(wrapper, { 'Nome da loja': 'Alta RPM', 'Link da loja': ' AltaRPM ' })

    expect(created).toEqual({ authorization: 'Bearer access-1', body: { name: 'Alta RPM', slug: 'altarpm' } })
    expect(switchedTo).toEqual({ tenantId: altaRpm.id })
    expect(loadPage).toHaveBeenCalledWith('/hoje')
  })

  it('shows where the store page will live once the link is valid', async () => {
    const { wrapper } = await openApp('/lojas')

    const address = field(wrapper, 'Link da loja')
    await address.setValue('altarpm')

    expect(wrapper.get(`#${address.attributes('aria-describedby')}`).text()).toBe('Sua loja ficará em altarpm.encomendo.com.br.')
  })

  it('says so next to the link when another store uses it', async () => {
    server.use(http.post('*/api/v1/admin/tenants', () => problem(409, 'slug_taken', 'Endereço em uso')))
    const { wrapper } = await openApp('/lojas')

    await submitForm(wrapper, { 'Nome da loja': 'Alta RPM', 'Link da loja': 'altarpm' })

    const address = field(wrapper, 'Link da loja')
    expect(address.attributes('aria-invalid')).toBe('true')
    expect(wrapper.text()).toContain('Este link já está em uso. Escolha outro.')
    expect(loadPage).not.toHaveBeenCalled()
  })

  it('asks for the missing fields', async () => {
    const { wrapper } = await openApp('/lojas')

    await submitForm(wrapper, { 'Nome da loja': '', 'Link da loja': '' })

    expect(wrapper.text()).toContain('Informe o nome da loja.')
    expect(wrapper.text()).toContain('Informe o link da loja.')
  })

  it('only retries the switch when the store was created but could not be opened', async () => {
    // Creating it again would say its own link is taken.
    let creates = 0
    let switches = 0
    server.use(
      http.post('*/api/v1/admin/tenants', () => {
        creates += 1
        return HttpResponse.json({ ...altaRpm, createdAt: '2026-11-25T12:00:00Z' }, { status: 201 })
      }),
      http.post('*/auth/switch-tenant', () => {
        switches += 1
        return switches === 1 ? problem(503, 'not_ready', 'Serviço indisponível') : HttpResponse.json(authSession())
      }),
    )
    const { wrapper } = await openApp('/lojas')

    await submitForm(wrapper, { 'Nome da loja': 'Alta RPM', 'Link da loja': 'altarpm' })
    expect(wrapper.get('[role=alert]').text()).toBe('O servidor não respondeu. Tente de novo em instantes.')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(creates).toBe(1)
    expect(switches).toBe(2)
    expect(loadPage).toHaveBeenCalledWith('/hoje')
  })

  it('lets the member log out from here', async () => {
    server.use(http.post('*/auth/logout', () => new HttpResponse(null, { status: 204 })))
    const { wrapper } = await openApp('/lojas')

    await button(wrapper, 'Sair').trigger('click')
    await flushPromises()

    expect(loadPage).toHaveBeenCalledWith('/entrar')
  })
})

describe('choosing a store', () => {
  const ownsAltaRpm = { tenant: altaRpm, role: 'owner' as const, status: 'active' as const }
  const both = [ownsAltaRpm, { tenant: motoPecas, role: 'staff' as const, status: 'active' as const }]

  it('lists the member’s stores and opens the chosen one where they were going', async () => {
    cookieHolds(storeless(both))
    let switchedTo: unknown
    server.use(
      http.post('*/auth/switch-tenant', async ({ request }) => {
        switchedTo = await request.json()
        return HttpResponse.json(authSession({ tenant: motoPecas, memberships: both }))
      }),
    )
    const { wrapper } = await openApp('/lojas?redirect=/hoje?mes=2026-11')

    expect(wrapper.get('h1').text()).toBe('Escolha a loja')
    await button(wrapper, 'Moto Peças Sul').trigger('click')
    await flushPromises()

    expect(switchedTo).toEqual({ tenantId: motoPecas.id })
    expect(loadPage).toHaveBeenCalledWith('/hoje?mes=2026-11')
  })

  it('still offers to create another store, as a secondary action', async () => {
    cookieHolds(storeless(both))
    const { wrapper } = await openApp('/lojas')

    expect(button(wrapper, 'Criar loja').classes()).toContain('enc-button--secondary')
  })

  it('says so when the member no longer belongs to the chosen store, and drops it from the list', async () => {
    server.use(
      // The reload's refresh still lists both; the one after the refusal only Alta RPM.
      http.post('*/auth/refresh', () => HttpResponse.json(storeless(both)), { once: true }),
      http.post('*/auth/refresh', () => HttpResponse.json(storeless([ownsAltaRpm]))),
      http.post('*/auth/switch-tenant', () => problem(403, 'forbidden', 'Sem acesso')),
    )
    const { wrapper } = await openApp('/lojas')

    await button(wrapper, 'Moto Peças Sul').trigger('click')
    await flushPromises()

    expect(wrapper.get('[role=alert]').text()).toBe('Você não faz mais parte desta loja.')
    expect(wrapper.findAll('button').map((b) => b.text())).not.toContain('Moto Peças Sul')
    expect(loadPage).not.toHaveBeenCalled()
  })

  it('sends the member to log in again when the session behind the switch is gone', async () => {
    server.use(
      http.post('*/auth/refresh', () => HttpResponse.json(storeless(both)), { once: true }),
      http.post('*/auth/refresh', () => problem(401, 'unauthenticated', 'Sessão expirada')),
      http.post('*/auth/switch-tenant', () => problem(401, 'unauthenticated', 'Sessão expirada')),
    )
    const { wrapper } = await openApp('/lojas')

    await button(wrapper, 'Moto Peças Sul').trigger('click')
    await flushPromises()

    expect(loadPage).toHaveBeenCalledWith('/entrar?redirect=/lojas')
  })

  it('offers to switch store from the menu when the member has more than one', async () => {
    cookieHolds(authSession({ memberships: both }))

    const { wrapper } = await openApp('/hoje')

    expect(link(wrapper, 'Trocar de loja').attributes('href')).toBe('/lojas')
  })

  it('offers no switch with a single store', async () => {
    cookieHolds(authSession())

    const { wrapper } = await openApp('/hoje')

    expect(wrapper.findAll('a').map((a) => a.text())).not.toContain('Trocar de loja')
  })
})

describe('a suspended store (ADR 0022)', () => {
  const suspendedMoto = { tenant: motoPecas, role: 'staff' as const, status: 'suspended' as const }
  const ownsAltaRpm = { tenant: altaRpm, role: 'owner' as const, status: 'active' as const }
  const message = 'Esta loja está suspensa. Fale com o suporte do Encomendo.'

  it('stays in the list marked "Suspensa", beside its link, and only that store is marked', async () => {
    cookieHolds(storeless([ownsAltaRpm, suspendedMoto]))

    const { wrapper } = await openApp('/lojas')

    const items = wrapper.findAll('li')
    expect(items.map((item) => item.text())).toEqual([
      expect.stringContaining('Alta RPM'),
      expect.stringContaining('Moto Peças Sul'),
    ])
    expect(items[0]?.text()).not.toContain('Suspensa')
    expect(items[1]?.text()).toContain('motopecas.encomendo.com.br')
    expect(items[1]?.text()).toContain('Suspensa')
    // The word is also what the button is described by, for screen readers.
    const described = items[1]?.get('button').attributes('aria-describedby')
    expect(wrapper.get(`#${described}`).text()).toBe('Suspensa')
  })

  it('refuses to enter it, says why and stays on the picker, without calling the API', async () => {
    cookieHolds(storeless([ownsAltaRpm, suspendedMoto]))
    let switches = 0
    server.use(
      http.post('*/auth/switch-tenant', () => {
        switches += 1
        return HttpResponse.json(authSession())
      }),
    )
    const { wrapper, router } = await openApp('/lojas')

    await button(wrapper, 'Moto Peças Sul').trigger('click')
    await flushPromises()

    expect(wrapper.get('[role=alert]').text()).toBe(message)
    expect(switches).toBe(0)
    expect(loadPage).not.toHaveBeenCalled()
    expect(router.currentRoute.value.name).toBe('stores')
  })

  it('says the same when the API still answers tenant_suspended to the switch, and marks the store afterwards', async () => {
    // The store was suspended after this list was read.
    server.use(
      http.post('*/auth/refresh', () => HttpResponse.json(storeless([ownsAltaRpm, { ...suspendedMoto, status: 'active' }])), { once: true }),
      http.post('*/auth/refresh', () => HttpResponse.json(storeless([ownsAltaRpm, suspendedMoto]))),
      http.post('*/auth/switch-tenant', () => problem(403, 'tenant_suspended', 'Qualquer título')),
    )
    const { wrapper } = await openApp('/lojas')

    await button(wrapper, 'Moto Peças Sul').trigger('click')
    await flushPromises()

    expect(wrapper.get('[role=alert]').text()).toBe(message)
    expect(wrapper.findAll('li')[1]?.text()).toContain('Suspensa')
    expect(loadPage).not.toHaveBeenCalled()
  })

  it('still enters the other stores', async () => {
    cookieHolds(storeless([ownsAltaRpm, suspendedMoto]))
    server.use(http.post('*/auth/switch-tenant', () => HttpResponse.json(authSession())))
    const { wrapper } = await openApp('/lojas')

    await button(wrapper, 'Alta RPM').trigger('click')
    await flushPromises()

    expect(loadPage).toHaveBeenCalledWith('/hoje')
  })

  it('says so when the member lands here from a store that was just suspended', async () => {
    cookieHolds(storeless([{ ...ownsAltaRpm, status: 'suspended' }]))

    const { wrapper } = await openApp('/lojas?suspended=1')

    expect(wrapper.get('h1').text()).toBe('Escolha a loja')
    expect(wrapper.get('[role=alert]').text()).toBe(message)
    expect(wrapper.get('li').text()).toContain('Suspensa')
  })

  it('shows nothing stale when the store was reactivated before the page reloaded', async () => {
    cookieHolds(storeless([ownsAltaRpm]))

    const { wrapper } = await openApp('/lojas?suspended=1')

    expect(wrapper.find('[role=alert]').exists()).toBe(false)
    expect(wrapper.get('li').text()).not.toContain('Suspensa')
  })

  it('does not open a remembered last store: a login with it lands on the picker', async () => {
    // The API answers a session without a store (ADR 0022); the app goes where that leads.
    cookieHolds(null)
    localStorage.setItem('encomendo.lastStore', altaRpm.id)
    let hint: unknown
    server.use(
      http.post('*/auth/login', async ({ request }) => {
        hint = await request.json()
        return HttpResponse.json(storeless([{ ...ownsAltaRpm, status: 'suspended' }]))
      }),
    )
    const { wrapper, router } = await openApp('/entrar')

    await submitForm(wrapper, { 'E-mail': 'ana@altarpm.com.br', Senha: 'segredo-forte' })
    await vi.waitFor(() => expect(router.currentRoute.value.name).toBe('stores'))
    await flushPromises()

    expect(hint).toMatchObject({ tenantId: altaRpm.id })
    expect(wrapper.get('h1').text()).toBe('Escolha a loja')
    expect(loadPage).not.toHaveBeenCalled()
  })
})
