import type { InvitationPreview } from '@encomendo/api'
import { enableAutoUnmount, flushPromises } from '@vue/test-utils'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'

import { loadPage } from '../../pageLoad'
import { button, closeApps, field, link, openApp, submitForm } from '../../testing/app'
import { altaRpm, authSession, problem } from '../../testing/fixtures'

vi.mock('../../pageLoad', () => ({ loadPage: vi.fn() }))

const server = setupServer()
beforeAll(() => server.listen({ onUnhandledFrame: 'error' }))
afterEach(() => {
  server.resetHandlers()
  vi.mocked(loadPage).mockClear()
})
afterAll(() => server.close())
enableAutoUnmount(afterEach)
afterEach(closeApps)

beforeEach(() => server.use(http.post('*/auth/refresh', () => problem(401, 'unauthenticated', 'Sessão expirada'))))

const invitation = (overrides: Partial<InvitationPreview> = {}): InvitationPreview => ({
  tenant: altaRpm,
  email: 'bruno@example.com',
  role: 'staff',
  accountExists: false,
  expiresAt: '2026-12-02T12:00:00Z',
  ...overrides,
})

/** openInvitation opens a link and waits until the preview has answered. */
async function openInvitation(path: string) {
  const app = await openApp(path)
  await vi.waitFor(() => expect(app.wrapper.text()).not.toContain('Verificando o convite…'))
  return app
}

/** previews answers the preview call, keeping what was asked. */
function previews(answer: () => Response) {
  const asked: unknown[] = []
  server.use(
    http.post('*/auth/invitations/preview', async ({ request }) => {
      asked.push(await request.json())
      return answer()
    }),
  )
  return asked
}

describe('accepting an invitation', () => {
  it('shows whose invitation it is and asks a new member for a name and a password', async () => {
    const asked = previews(() => HttpResponse.json(invitation()))

    const { wrapper } = await openInvitation('/convite#token=tok-123')

    expect(asked).toEqual([{ token: 'tok-123' }])
    expect(wrapper.get('h1').text()).toBe('Entrar na equipe da loja Alta RPM')
    expect(wrapper.text()).toContain('Convite para bruno@example.com.')
    expect(wrapper.findAll('label').map((l) => l.text())).toEqual(['Nome', 'Senha'])
    expect(field(wrapper, 'Senha').attributes('autocomplete')).toBe('new-password')
  })

  it('joins the store and opens it', async () => {
    previews(() => HttpResponse.json(invitation()))
    let body: unknown
    server.use(
      http.post('*/auth/invitations/accept', async ({ request }) => {
        body = await request.json()
        return HttpResponse.json(authSession())
      }),
    )
    const { wrapper } = await openInvitation('/convite#token=tok-123')

    await submitForm(wrapper, { Nome: ' Bruno Lima ', Senha: 'segredo-forte' })

    expect(body).toEqual({ token: 'tok-123', name: 'Bruno Lima', password: 'segredo-forte' })
    expect(loadPage).toHaveBeenCalledWith('/hoje')
  })

  it('asks an existing account only for its password', async () => {
    previews(() => HttpResponse.json(invitation({ accountExists: true })))
    let body: unknown
    server.use(
      http.post('*/auth/invitations/accept', async ({ request }) => {
        body = await request.json()
        return HttpResponse.json(authSession())
      }),
    )
    const { wrapper } = await openInvitation('/convite#token=tok-123')

    expect(wrapper.text()).toContain('Você já tem uma conta com bruno@example.com. Entre com a sua senha para aceitar.')
    expect(wrapper.findAll('label').map((l) => l.text())).toEqual(['Senha'])
    expect(field(wrapper, 'Senha').attributes('autocomplete')).toBe('current-password')
    expect(link(wrapper, 'Esqueci a senha').attributes('href')).toBe('/esqueci-a-senha')
    await submitForm(wrapper, { Senha: 'segredo-forte' })
    expect(body).toEqual({ token: 'tok-123', password: 'segredo-forte' })
  })

  it('says so next to the password when it is wrong', async () => {
    previews(() => HttpResponse.json(invitation({ accountExists: true })))
    server.use(http.post('*/auth/invitations/accept', () => problem(401, 'invalid_credentials', 'Senha incorreta')))
    const { wrapper } = await openInvitation('/convite#token=tok-123')

    await submitForm(wrapper, { Senha: 'errada' })

    const password = field(wrapper, 'Senha')
    expect(password.attributes('aria-invalid')).toBe('true')
    expect(wrapper.text()).toContain('Senha incorreta.')
    expect(loadPage).not.toHaveBeenCalled()
  })

  it('says so when the member already belongs to the store, and offers to log in', async () => {
    previews(() => HttpResponse.json(invitation({ accountExists: true })))
    server.use(http.post('*/auth/invitations/accept', () => problem(409, 'already_member', 'Já é membro')))
    const { wrapper } = await openInvitation('/convite#token=tok-123')

    await submitForm(wrapper, { Senha: 'segredo-forte' })

    expect(wrapper.get('[role=alert]').text()).toBe('Você já faz parte desta loja.')
    expect(link(wrapper, 'Entrar').attributes('href')).toBe('/entrar')
  })

  it('explains an invitation that no longer works', async () => {
    previews(() => problem(404, 'not_found', 'Não encontrado'))

    const { wrapper } = await openInvitation('/convite#token=tok-old')

    expect(wrapper.get('h1').text()).toBe('Convite indisponível')
    expect(wrapper.text()).toContain('Este convite não vale mais. Peça um novo a quem convidou você.')
    expect(wrapper.find('form').exists()).toBe(false)
  })

  it('asks to open the email link again when the link has no token, without asking the API', async () => {
    // As after logging in again from this page: the token never travels in a redirect.
    const asked = previews(() => HttpResponse.json(invitation()))

    const { wrapper } = await openInvitation('/convite')

    expect(asked).toEqual([])
    expect(wrapper.get('h1').text()).toBe('Abra o convite pelo e-mail')
    expect(wrapper.text()).toContain('Este endereço não traz o convite. Abra de novo o link que chegou no seu e-mail.')
  })

  it('shows the unavailable invitation when it ran out between opening and accepting', async () => {
    previews(() => HttpResponse.json(invitation()))
    server.use(http.post('*/auth/invitations/accept', () => problem(404, 'not_found', 'Não encontrado')))
    const { wrapper } = await openInvitation('/convite#token=tok-123')

    await submitForm(wrapper, { Nome: 'Bruno Lima', Senha: 'segredo-forte' })

    expect(wrapper.get('h1').text()).toBe('Convite indisponível')
    expect(wrapper.find('form').exists()).toBe(false)
  })

  it('opens the new invitation when another link is pasted into the same tab', async () => {
    const asked = previews(() => HttpResponse.json(invitation()))
    const { wrapper, router } = await openInvitation('/convite#token=tok-first')

    await router.push('/convite#token=tok-second')
    await flushPromises()

    expect(asked).toEqual([{ token: 'tok-first' }, { token: 'tok-second' }])
    expect(wrapper.get('h1').text()).toBe('Entrar na equipe da loja Alta RPM')
  })

  it('offers to try again when the server cannot be reached', async () => {
    let calls = 0
    previews(() => {
      calls += 1
      return calls === 1 ? HttpResponse.error() : HttpResponse.json(invitation())
    })
    const { wrapper } = await openInvitation('/convite#token=tok-123')

    expect(wrapper.get('[role=alert]').text()).toBe('Sem conexão com o servidor. Confira a internet e tente de novo.')
    await button(wrapper, 'Tentar de novo').trigger('click')
    await flushPromises()

    expect(wrapper.get('h1').text()).toBe('Entrar na equipe da loja Alta RPM')
  })
})
