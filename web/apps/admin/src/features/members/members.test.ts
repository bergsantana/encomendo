import type { Invitation, Member, Permission } from '@encomendo/api'
import { enableAutoUnmount, flushPromises, type VueWrapper } from '@vue/test-utils'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'

import { button, closeApps, field, link, openApp, submitForm } from '../../testing/app'
import { loadPage } from '../../pageLoad'
import { altaRpm, authSession, gate, problem } from '../../testing/fixtures'

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

const manage: Permission[] = ['order:read', 'members:manage']
const ana: Member = { userId: authSession().user.id, name: 'Ana Souza', email: 'ana@altarpm.com.br', role: 'owner', joinedAt: '2026-11-01T12:00:00Z' }
const bruno: Member = {
  userId: '0190f3c4-5e1a-7b2c-8d3e-00000000beef',
  name: 'Bruno Lima',
  email: 'bruno@example.com',
  role: 'staff',
  joinedAt: '2026-11-20T12:00:00Z',
}
const carla: Invitation = {
  id: '0190f3c4-5e1a-7b2c-8d3e-00000000a11a',
  email: 'carla@example.com',
  role: 'viewer',
  invitedAt: '2026-11-25T12:00:00Z',
  expiresAt: '2026-12-02T12:00:00Z',
}

/** The signed-in member, as the refresh cookie restores them. */
const signedInAs = (role: Member['role'], permissions: Permission[]) =>
  server.use(http.post('*/auth/refresh', () => HttpResponse.json(authSession({ role, permissions }))))

/** The team and the pending invitations, as the API lists them. */
function team(members: Member[] = [ana, bruno], invitations: Invitation[] = [carla]) {
  server.use(
    http.get('*/api/v1/admin/members', () => HttpResponse.json({ items: members })),
    http.get('*/api/v1/admin/invitations', () => HttpResponse.json({ items: invitations })),
  )
}

beforeEach(() => server.use(http.get('*/readyz', () => HttpResponse.json({ status: 'ok' }))))

/** row is the table row that names someone. */
function row(wrapper: VueWrapper, text: string) {
  const found = wrapper.findAll('tr').find((tr) => tr.text().includes(text))
  if (found === undefined) throw new Error(`no row for ${text}`)
  return found
}

async function openTeam(invited?: string) {
  const app = await openApp('/equipe')
  await vi.waitFor(() => expect(app.wrapper.text()).toContain('Bruno Lima'))
  if (invited !== undefined) await vi.waitFor(() => expect(app.wrapper.text()).toContain(invited))
  return app
}

describe('the Equipe screen', () => {
  it('is in the menu only for who can manage the team', async () => {
    signedInAs('owner', manage)
    team()
    const { wrapper } = await openApp('/hoje')

    expect(link(wrapper, 'Equipe').attributes('href')).toBe('/equipe')
  })

  it('is hidden from roles that cannot manage the team, and its address leads to Hoje', async () => {
    signedInAs('viewer', ['order:read'])

    const { wrapper, router } = await openApp('/equipe')

    expect(router.currentRoute.value.name).toBe('hoje')
    expect(wrapper.findAll('a').map((a) => a.text())).not.toContain('Equipe')
  })

  it('lists the team with their roles, and leaves the owner as is', async () => {
    signedInAs('owner', manage)
    team()

    const { wrapper } = await openTeam()

    expect(row(wrapper, 'Ana Souza').text()).toContain('(você)')
    expect(row(wrapper, 'Ana Souza').text()).toContain('Dono')
    expect(row(wrapper, 'Ana Souza').find('select').exists()).toBe(false)
    expect(row(wrapper, 'Ana Souza').findAll('button')).toHaveLength(0)
    const brunoRole = row(wrapper, 'Bruno Lima').get('select')
    expect((brunoRole.element as HTMLSelectElement).value).toBe('staff')
    expect(brunoRole.findAll('option').map((o) => o.text())).toEqual(['Admin', 'Funcionário', 'Visualizador'])
  })

  it('changes a role only when asked, not on every pick', async () => {
    // Arrow keys on a closed select change it step by step; each step must not be a save.
    signedInAs('owner', manage)
    team()
    const bodies: unknown[] = []
    server.use(
      http.put(`*/api/v1/admin/members/${bruno.userId}/role`, async ({ request }) => {
        bodies.push(await request.json())
        return HttpResponse.json({ ...bruno, role: 'viewer' })
      }),
    )
    const { wrapper } = await openTeam()

    await row(wrapper, 'Bruno Lima').get('select').setValue('admin')
    await row(wrapper, 'Bruno Lima').get('select').setValue('viewer')
    await flushPromises()
    expect(bodies).toEqual([])
    await button(row(wrapper, 'Bruno Lima'), 'Alterar papel de Bruno Lima').trigger('click')
    await flushPromises()

    expect(bodies).toEqual([{ role: 'viewer' }])
    const status = wrapper.get('[role=status]')
    expect(status.text()).toBe('Bruno Lima agora é Visualizador.')
    // "Alterar papel" is gone once saved; focus lands on what happened.
    expect(document.activeElement).toBe(status.element)
  })

  it('offers to save a role only once it differs', async () => {
    signedInAs('owner', manage)
    team()
    const { wrapper } = await openTeam()

    expect(row(wrapper, 'Bruno Lima').text()).not.toContain('Alterar papel')
    await row(wrapper, 'Bruno Lima').get('select').setValue('admin')

    expect(row(wrapper, 'Bruno Lima').text()).toContain('Alterar papel')
  })

  it('keeps the role choice usable while the change is saved', async () => {
    signedInAs('owner', manage)
    team()
    const saving = gate()
    server.use(
      http.put(`*/api/v1/admin/members/${bruno.userId}/role`, async () => {
        await saving.opened
        return HttpResponse.json({ ...bruno, role: 'admin' })
      }),
    )
    const { wrapper } = await openTeam()

    await row(wrapper, 'Bruno Lima').get('select').setValue('admin')
    await button(row(wrapper, 'Bruno Lima'), 'Alterar papel de Bruno Lima').trigger('click')

    expect(row(wrapper, 'Bruno Lima').get('select').attributes('disabled')).toBeUndefined()
    expect(button(row(wrapper, 'Bruno Lima'), 'Alterar papel de Bruno Lima').attributes('disabled')).toBeDefined()
    saving.open()
    await flushPromises()
  })

  it('shows a failed change in its row', async () => {
    signedInAs('owner', manage)
    team()
    server.use(http.put(`*/api/v1/admin/members/${bruno.userId}/role`, () => problem(503, 'not_ready', 'Indisponível')))
    const { wrapper } = await openTeam()

    await row(wrapper, 'Bruno Lima').get('select').setValue('admin')
    await button(row(wrapper, 'Bruno Lima'), 'Alterar papel de Bruno Lima').trigger('click')
    await flushPromises()

    expect(row(wrapper, 'Bruno Lima').get('[role=alert]').text()).toBe('O servidor não respondeu. Tente de novo em instantes.')
  })

  it('removes someone only after confirming', async () => {
    signedInAs('owner', manage)
    team()
    let removed = 0
    server.use(
      http.delete(`*/api/v1/admin/members/${bruno.userId}`, () => {
        removed += 1
        return new HttpResponse(null, { status: 204 })
      }),
    )
    const { wrapper } = await openTeam()

    await button(row(wrapper, 'Bruno Lima'), 'Remover Bruno Lima').trigger('click')
    expect(row(wrapper, 'Bruno Lima').text()).toContain('Remover Bruno Lima da equipe?')
    expect(removed).toBe(0)
    await button(wrapper, 'Confirmar remoção').trigger('click')
    await flushPromises()

    expect(removed).toBe(1)
    expect(wrapper.text()).not.toContain('bruno@example.com')
    const status = wrapper.get('[role=status]')
    expect(status.text()).toBe('Bruno Lima saiu da equipe.')
    // The row and its buttons are gone; focus lands on what happened.
    expect(document.activeElement).toBe(status.element)
  })

  it('reads the question out with the buttons that answer it', async () => {
    // Focus lands on "Cancelar"; a screen reader must also hear what is being asked.
    signedInAs('admin', manage)
    team([{ ...ana, role: 'admin' }, bruno])
    const { wrapper } = await openTeam()

    await row(wrapper, 'Ana Souza').get('select').setValue('staff')
    await button(row(wrapper, 'Ana Souza'), 'Alterar papel de Ana Souza').trigger('click')

    for (const name of ['Cancelar', 'Confirmar']) {
      const answer = button(row(wrapper, 'Ana Souza'), name)
      expect(wrapper.get(`#${answer.attributes('aria-describedby')}`).text()).toBe('Você deixará de gerenciar a equipe.')
    }
    expect(document.activeElement).toBe(button(row(wrapper, 'Ana Souza'), 'Cancelar').element)
  })

  it('puts the role back and focus on its choice when the own-role question is cancelled', async () => {
    signedInAs('admin', manage)
    team([{ ...ana, role: 'admin' }, bruno])
    const { wrapper } = await openTeam()

    await row(wrapper, 'Ana Souza').get('select').setValue('staff')
    await button(row(wrapper, 'Ana Souza'), 'Alterar papel de Ana Souza').trigger('click')
    await button(row(wrapper, 'Ana Souza'), 'Cancelar').trigger('click')
    await flushPromises()

    const select = row(wrapper, 'Ana Souza').get('select')
    expect((select.element as HTMLSelectElement).value).toBe('admin')
    expect(row(wrapper, 'Ana Souza').text()).not.toContain('Alterar papel')
    expect(document.activeElement).toBe(select.element)
  })

  it('returns focus to "Remover" when the removal is cancelled', async () => {
    signedInAs('owner', manage)
    team()
    const { wrapper } = await openTeam()

    await row(wrapper, 'Bruno Lima').get('select').setValue('admin')
    await button(row(wrapper, 'Bruno Lima'), 'Remover Bruno Lima').trigger('click')
    await button(row(wrapper, 'Bruno Lima'), 'Cancelar').trigger('click')
    await flushPromises()

    expect(document.activeElement).toBe(button(row(wrapper, 'Bruno Lima'), 'Remover Bruno Lima').element)
  })

  it('warns the member removing themselves that they lose the store', async () => {
    signedInAs('admin', manage)
    team([{ ...ana, role: 'admin' }, bruno])
    const { wrapper } = await openTeam()

    await button(row(wrapper, 'Ana Souza'), 'Remover Ana Souza').trigger('click')

    expect(row(wrapper, 'Ana Souza').text()).toContain('Você sairá da equipe e perderá o acesso a esta loja.')
  })

  it('keeps the member when the removal is cancelled', async () => {
    signedInAs('owner', manage)
    team()
    const { wrapper } = await openTeam()

    await button(row(wrapper, 'Bruno Lima'), 'Remover Bruno Lima').trigger('click')
    await button(wrapper, 'Cancelar').trigger('click')

    expect(row(wrapper, 'Bruno Lima').text()).not.toContain('da equipe?')
    expect(button(row(wrapper, 'Bruno Lima'), 'Remover Bruno Lima').exists()).toBe(true)
  })

  it('says so when the person already left, and shows the team as it is now', async () => {
    signedInAs('owner', manage)
    let reads = 0
    server.use(
      http.get('*/api/v1/admin/members', () => {
        reads += 1
        return HttpResponse.json({ items: reads === 1 ? [ana, bruno] : [ana] })
      }),
      http.get('*/api/v1/admin/invitations', () => HttpResponse.json({ items: [] })),
      http.put(`*/api/v1/admin/members/${bruno.userId}/role`, () => problem(404, 'not_found', 'Não encontrado')),
    )
    const { wrapper } = await openTeam()

    await row(wrapper, 'Bruno Lima').get('select').setValue('viewer')
    await button(row(wrapper, 'Bruno Lima'), 'Alterar papel de Bruno Lima').trigger('click')
    await flushPromises()

    await vi.waitFor(() => expect(wrapper.text()).not.toContain('bruno@example.com'))
    expect(wrapper.get('[role=alert]').text()).toBe('Esta pessoa não faz mais parte da equipe.')
  })

  it('takes the member out of the screen when changing their own role takes away managing the team', async () => {
    const admin = { ...ana, role: 'admin' as const }
    let refreshes = 0
    server.use(
      http.post('*/auth/refresh', () => {
        refreshes += 1
        return HttpResponse.json(refreshes === 1 ? authSession({ role: 'admin', permissions: manage }) : authSession({ role: 'staff', permissions: ['order:read'] }))
      }),
      http.get('*/api/v1/admin/members', () => HttpResponse.json({ items: [admin, bruno] })),
      http.get('*/api/v1/admin/invitations', () => HttpResponse.json({ items: [] })),
      http.put(`*/api/v1/admin/members/${ana.userId}/role`, () => HttpResponse.json({ ...admin, role: 'staff' })),
    )
    const { wrapper, router } = await openTeam()

    await row(wrapper, 'Ana Souza').get('select').setValue('staff')
    await button(row(wrapper, 'Ana Souza'), 'Alterar papel de Ana Souza').trigger('click')
    // Their own role: it asks first, since they will stop managing the team.
    expect(row(wrapper, 'Ana Souza').text()).toContain('Você deixará de gerenciar a equipe.')
    expect(refreshes).toBe(1)
    await button(row(wrapper, 'Ana Souza'), 'Confirmar').trigger('click')
    await flushPromises()

    await vi.waitFor(() => expect(refreshes).toBe(2))
    await vi.waitFor(() => expect(router.currentRoute.value.name).toBe('hoje'))
  })

  it('loads more of the team', async () => {
    signedInAs('owner', manage)
    server.use(
      http.get('*/api/v1/admin/members', ({ request }) =>
        new URL(request.url).searchParams.get('cursor') === 'page-2'
          ? HttpResponse.json({ items: [bruno] })
          : HttpResponse.json({ items: [ana], nextCursor: 'page-2' }),
      ),
      http.get('*/api/v1/admin/invitations', () => HttpResponse.json({ items: [] })),
    )
    const { wrapper } = await openApp('/equipe')
    await vi.waitFor(() => expect(wrapper.text()).toContain('Ana Souza'))

    await button(wrapper, 'Carregar mais').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('Bruno Lima')
    expect(wrapper.findAll('button').map((b) => b.text())).not.toContain('Carregar mais')
  })
})

describe('invitations', () => {
  beforeEach(() => signedInAs('owner', manage))

  it('lists who was invited, with the role and until when the link works', async () => {
    team()

    const { wrapper } = await openTeam('carla@example.com')

    const invited = row(wrapper, 'carla@example.com')
    expect(invited.text()).toContain('Visualizador')
    expect(invited.text()).toContain('02/12/2026')
  })

  it('invites by email with a role, and lists the invitation', async () => {
    team([ana, bruno], [])
    let body: unknown
    server.use(
      http.post('*/api/v1/admin/invitations', async ({ request }) => {
        body = await request.json()
        return HttpResponse.json(carla, { status: 201 })
      }),
    )
    const { wrapper } = await openTeam()

    await field(wrapper, 'Papel').setValue('viewer')
    await submitForm(wrapper, { 'E-mail': ' carla@example.com ' })

    expect(body).toEqual({ email: 'carla@example.com', role: 'viewer' })
    expect(wrapper.get('[role=status]').text()).toBe('Convite enviado para carla@example.com.')
    expect(row(wrapper, 'carla@example.com').exists()).toBe(true)
    expect((field(wrapper, 'E-mail').element as HTMLInputElement).value).toBe('')
  })

  it('invites as Funcionário unless another role is chosen', async () => {
    team([ana], [])
    let body: unknown
    server.use(
      http.post('*/api/v1/admin/invitations', async ({ request }) => {
        body = await request.json()
        return HttpResponse.json(carla, { status: 201 })
      }),
    )
    const { wrapper } = await openApp('/equipe')
    await vi.waitFor(() => expect(wrapper.text()).toContain('Ana Souza'))

    await submitForm(wrapper, { 'E-mail': 'carla@example.com' })

    expect(body).toMatchObject({ role: 'staff' })
  })

  it('says so next to the email when the person is already on the team', async () => {
    team()
    server.use(http.post('*/api/v1/admin/invitations', () => problem(409, 'already_member', 'Já é membro')))
    const { wrapper } = await openTeam()

    await submitForm(wrapper, { 'E-mail': 'bruno@example.com' })

    expect(wrapper.text()).toContain('Esta pessoa já faz parte da equipe.')
    expect(document.activeElement).toBe(field(wrapper, 'E-mail').element)
  })

  it('says the invitation was saved when the email could not go out, and lists it', async () => {
    let reads = 0
    server.use(
      http.get('*/api/v1/admin/members', () => HttpResponse.json({ items: [ana, bruno] })),
      http.get('*/api/v1/admin/invitations', () => {
        reads += 1
        return HttpResponse.json({ items: reads === 1 ? [] : [carla] })
      }),
      http.post('*/api/v1/admin/invitations', () => problem(503, 'email_unavailable', 'E-mail indisponível')),
    )
    const { wrapper } = await openTeam()

    await submitForm(wrapper, { 'E-mail': 'carla@example.com' })

    expect(wrapper.get('[role=alert]').text()).toBe('O convite foi salvo, mas o e-mail não saiu. Use "Reenviar" para tentar de novo.')
    expect(row(wrapper, 'carla@example.com').exists()).toBe(true)
  })

  it('sends the invitation again, with a new link', async () => {
    team()
    let body: unknown
    server.use(
      http.post('*/api/v1/admin/invitations', async ({ request }) => {
        body = await request.json()
        return HttpResponse.json({ ...carla, expiresAt: '2026-12-09T12:00:00Z' }, { status: 201 })
      }),
    )
    const { wrapper } = await openTeam('carla@example.com')

    await button(row(wrapper, 'carla@example.com'), 'Reenviar convite para carla@example.com').trigger('click')
    await flushPromises()

    expect(body).toEqual({ email: 'carla@example.com', role: 'viewer' })
    const status = wrapper.get('[role=status]')
    expect(status.text()).toBe('Convite reenviado para carla@example.com.')
    expect(document.activeElement).toBe(status.element)
    expect(row(wrapper, 'carla@example.com').text()).toContain('09/12/2026')
  })

  it('cancels an invitation', async () => {
    team()
    let cancelled = false
    server.use(
      http.delete(`*/api/v1/admin/invitations/${carla.id}`, () => {
        cancelled = true
        return new HttpResponse(null, { status: 204 })
      }),
    )
    const { wrapper } = await openTeam('carla@example.com')

    await button(row(wrapper, 'carla@example.com'), 'Cancelar convite para carla@example.com').trigger('click')
    await flushPromises()

    expect(cancelled).toBe(true)
    expect(wrapper.text()).not.toContain('carla@example.com.br')
    expect(wrapper.findAll('tr').some((tr) => tr.text().includes('carla@example.com'))).toBe(false)
    expect(wrapper.get('[role=status]').text()).toBe('Convite para carla@example.com cancelado.')
  })
})

describe('invitation edge cases', () => {
  beforeEach(() => signedInAs('owner', manage))

  it('says the link was replaced when "Reenviar" could not send the email, and reads the list again', async () => {
    let invitationReads = 0
    server.use(
      http.get('*/api/v1/admin/members', () => HttpResponse.json({ items: [ana, bruno] })),
      http.get('*/api/v1/admin/invitations', () => {
        invitationReads += 1
        return HttpResponse.json({ items: [invitationReads === 1 ? carla : { ...carla, expiresAt: '2026-12-09T12:00:00Z' }] })
      }),
      http.post('*/api/v1/admin/invitations', () => problem(503, 'email_unavailable', 'E-mail indisponível')),
    )
    const { wrapper } = await openTeam('carla@example.com')

    await button(row(wrapper, 'carla@example.com'), 'Reenviar convite para carla@example.com').trigger('click')

    await vi.waitFor(() => expect(row(wrapper, 'carla@example.com').text()).toContain('09/12/2026'))
    expect(row(wrapper, 'carla@example.com').get('[role=alert]').text()).toBe(
      'O convite foi salvo, mas o e-mail não saiu. Use "Reenviar" para tentar de novo.',
    )
  })

  it('says so when the invited person joined meanwhile, and shows the team as it is now', async () => {
    let memberReads = 0
    let invitationReads = 0
    const joined: Member = {
      userId: '0190f3c4-5e1a-7b2c-8d3e-0000000ca71a',
      name: 'Carla Dias',
      email: carla.email,
      role: 'viewer',
      joinedAt: '2026-11-26T12:00:00Z',
    }
    server.use(
      http.get('*/api/v1/admin/members', () => {
        memberReads += 1
        return HttpResponse.json({ items: memberReads === 1 ? [ana, bruno] : [ana, bruno, joined] })
      }),
      http.get('*/api/v1/admin/invitations', () => {
        invitationReads += 1
        return HttpResponse.json({ items: invitationReads === 1 ? [carla] : [] })
      }),
      http.post('*/api/v1/admin/invitations', () => problem(409, 'already_member', 'Já é membro')),
    )
    const { wrapper } = await openTeam('carla@example.com')

    await button(row(wrapper, 'carla@example.com'), 'Reenviar convite para carla@example.com').trigger('click')

    await vi.waitFor(() => expect(wrapper.text()).toContain('Carla Dias'))
    expect(wrapper.get('[role=alert]').text()).toBe('Esta pessoa já faz parte da equipe.')
  })

  it('replaces the pending invitation of the same email whatever its case', async () => {
    team([ana, bruno], [carla])
    server.use(http.post('*/api/v1/admin/invitations', () => HttpResponse.json({ ...carla, email: 'Carla@Example.com' }, { status: 201 })))
    const { wrapper } = await openTeam('carla@example.com')

    await submitForm(wrapper, { 'E-mail': 'Carla@Example.com' })

    expect(wrapper.findAll('tr').filter((tr) => tr.text().toLowerCase().includes('carla@example.com'))).toHaveLength(1)
  })
})

describe('when the team cannot be read', () => {
  it('says why and offers to try again', async () => {
    signedInAs('owner', manage)
    let reads = 0
    server.use(
      http.get('*/api/v1/admin/members', () => {
        reads += 1
        return reads === 1 ? HttpResponse.error() : HttpResponse.json({ items: [ana, bruno] })
      }),
      http.get('*/api/v1/admin/invitations', () => HttpResponse.json({ items: [] })),
    )
    const { wrapper } = await openApp('/equipe')
    await vi.waitFor(() => expect(wrapper.find('[role=alert]').exists()).toBe(true))

    expect(wrapper.get('[role=alert]').text()).toBe('Sem conexão com o servidor. Confira a internet e tente de novo.')
    await button(wrapper, 'Tentar de novo').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('Bruno Lima')
  })

  it('says so when the role no longer allows managing the team', async () => {
    signedInAs('owner', manage)
    server.use(
      http.get('*/api/v1/admin/members', () => problem(403, 'forbidden', 'Sem permissão')),
      http.get('*/api/v1/admin/invitations', () => problem(403, 'forbidden', 'Sem permissão')),
    )
    const { wrapper } = await openApp('/equipe')
    await vi.waitFor(() => expect(wrapper.find('[role=alert]').exists()).toBe(true))

    expect(wrapper.get('[role=alert]').text()).toBe('Você não tem mais permissão para gerenciar a equipe.')
  })

  it('goes back to the store picker, saying so, when the store is suspended', async () => {
    // The same for every screen in the store: the HTTP client reports it (ADR 0022).
    signedInAs('owner', manage)
    server.use(
      http.get('*/api/v1/admin/members', () => problem(403, 'tenant_suspended', 'Esta loja está suspensa. Fale com o suporte do Encomendo.')),
      http.get('*/api/v1/admin/invitations', () => problem(403, 'tenant_suspended', 'Esta loja está suspensa. Fale com o suporte do Encomendo.')),
    )

    await openApp('/equipe')
    await vi.waitFor(() => expect(loadPage).toHaveBeenCalledWith('/lojas?suspended=1'))
  })

  it('says so when the access token expires and the refresh finds the store suspended', async () => {
    // The refreshed session has no store and marks it (ADR 0022): not "replaced by another login".
    const suspended = { tenant: altaRpm, role: 'owner' as const, status: 'suspended' as const }
    server.use(
      http.post('*/auth/refresh', () => HttpResponse.json(authSession({ role: 'owner', permissions: manage })), { once: true }),
      http.post('*/auth/refresh', () => HttpResponse.json(authSession({ tenant: undefined, role: undefined, permissions: [], memberships: [suspended] }))),
      http.get('*/api/v1/admin/members', () => problem(401, 'unauthenticated', 'Sessão expirada')),
      http.get('*/api/v1/admin/invitations', () => problem(401, 'unauthenticated', 'Sessão expirada')),
    )

    await openApp('/equipe')

    await vi.waitFor(() => expect(loadPage).toHaveBeenCalledWith('/lojas?suspended=1'))
    expect(loadPage).not.toHaveBeenCalledWith(expect.stringContaining('/entrar'))
  })

  it('leaves for Hoje when a change is refused because the role changed meanwhile', async () => {
    let refreshes = 0
    server.use(
      http.post('*/auth/refresh', () => {
        refreshes += 1
        return HttpResponse.json(
          refreshes === 1 ? authSession({ role: 'admin', permissions: manage }) : authSession({ role: 'staff', permissions: ['order:read'] }),
        )
      }),
      http.get('*/api/v1/admin/members', () => HttpResponse.json({ items: [{ ...ana, role: 'admin' }, bruno] })),
      http.get('*/api/v1/admin/invitations', () => HttpResponse.json({ items: [] })),
      http.delete(`*/api/v1/admin/members/${bruno.userId}`, () => problem(403, 'forbidden', 'Sem permissão')),
    )
    const { wrapper, router } = await openTeam()

    await button(row(wrapper, 'Bruno Lima'), 'Remover Bruno Lima').trigger('click')
    await button(wrapper, 'Confirmar remoção').trigger('click')
    await flushPromises()

    await vi.waitFor(() => expect(router.currentRoute.value.name).toBe('hoje'))
  })
})
