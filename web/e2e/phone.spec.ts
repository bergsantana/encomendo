import { expect, test, type Locator, type Page } from '@playwright/test'

import { createStore, latestEmail, logOut, mailpitUrl, openMenu, password, signUp, unique } from './support'

// The admin's phone layouts (ADR 0029, step 3.5.3), in the `phone` project
// of playwright.config.ts: 390 x 844, touch, no hover. It follows the same
// rules as admin.spec.ts: the first describe runs against `vite preview`,
// the second against the real stack (`make stack-up`, E2E_BASE_URL and
// E2E_MAILPIT_URL). This file signs up three accounts per run, which counts
// against the same 10-an-hour-per-IP limit; see the note in admin.spec.ts.
const realStack = process.env.E2E_BASE_URL !== undefined && process.env.E2E_BASE_URL !== ''

/** expectNoSideScroll fails when the page is wider than the screen: a phone must never scroll sideways. */
async function expectNoSideScroll(page: Page): Promise<void> {
  const widths = await page.evaluate(() => ({ scroll: document.documentElement.scrollWidth, inner: window.innerWidth }))
  expect(widths.scroll, `page is ${widths.scroll} px wide on a ${widths.inner} px screen`).toBeLessThanOrEqual(widths.inner)
}

/** expectThumbSized checks the controls a thumb must hit are at least 48 px tall. */
async function expectThumbSized(...controls: Locator[]): Promise<void> {
  for (const control of controls) {
    const box = await control.boundingBox()
    expect(box?.height ?? 0, await control.evaluate((element) => element.outerHTML.slice(0, 80))).toBeGreaterThanOrEqual(48)
  }
}

test.describe('without an API', () => {
  test.skip(realStack, 'runs only against `vite preview`, which has no API')

  const store = { id: '0190f3c4-5e1a-7b2c-8d3e-4f5a6b7c8d00', name: 'Alta RPM', slug: 'altarpm' }
  const refused = { type: 'about:blank', title: 'Sessão expirada', status: 401, code: 'unauthenticated' }
  const ana = { id: '0190f3c4-5e1a-7b2c-8d3e-4f5a6b7c8d9e', name: 'Ana Souza', email: 'ana@altarpm.com.br' }
  // Two stores, so the drawer offers "Trocar de loja" and the picker has a list.
  const session = (permissions: string[]) => ({
    accessToken: 'e2e-access',
    tokenType: 'Bearer',
    expiresIn: 900,
    user: ana,
    tenant: store,
    role: 'owner',
    permissions,
    memberships: [
      { tenant: store, role: 'owner', status: 'active' },
      { tenant: { ...store, id: '0190f3c4-5e1a-7b2c-8d3e-4f5a6b7c8d01', name: 'Outra Loja', slug: 'outra' }, role: 'admin', status: 'active' },
    ],
  })

  test.beforeEach(async ({ page }) => {
    await page.route('**/auth/refresh', (route) => route.fulfill({ status: 401, contentType: 'application/problem+json', body: JSON.stringify(refused) }))
    await page.route('**/auth/login', (route) => route.fulfill({ json: session(['order:read', 'order:write']) }))
  })

  // Signed in from the start, with a team and invitations the API would send, so CI
  // (which has no real stack) still checks the cards, the picker and long emails.
  test.describe('signed in', () => {
    const longEmail = `${'a'.repeat(34)}@${'b'.repeat(21)}.com`
    const members = [
      { userId: ana.id, name: ana.name, email: ana.email, role: 'owner', joinedAt: '2026-11-01T12:00:00Z' },
      { userId: '0190f3c4-5e1a-7b2c-8d3e-00000000beef', name: 'Bruno Lima', email: longEmail, role: 'staff', joinedAt: '2026-11-02T12:00:00Z' },
    ]
    const invitations = [
      { id: '0190f3c4-5e1a-7b2c-8d3e-00000000cafe', email: `convite-${longEmail}`, role: 'viewer', expiresAt: '2026-12-04T12:00:00Z', createdAt: '2026-11-27T12:00:00Z' },
    ]

    test.beforeEach(async ({ page }) => {
      await page.route('**/auth/refresh', (route) => route.fulfill({ json: session(['order:read', 'members:manage']) }))
      await page.route('**/api/v1/admin/members*', (route) => route.fulfill({ json: { items: members } }))
      await page.route('**/api/v1/admin/invitations*', (route) => route.fulfill({ json: { items: invitations } }))
    })

    test('Equipe shows cards: no side scroll, long emails, named actions, thumb-sized targets', async ({ page }) => {
      expect(longEmail).toHaveLength(60)
      await page.goto('/equipe')
      await expect(page.getByRole('heading', { name: 'Equipe', exact: true })).toBeVisible()
      await expect(page.getByText(longEmail, { exact: true })).toBeVisible()

      // A row showing display: block proves the card styles applied; a table row would be table-row.
      const display = await page.getByRole('row', { name: /Bruno Lima/ }).evaluate((row) => getComputedStyle(row).display)
      expect(display).not.toBe('table-row')
      await expect(page.locator('tbody tr').first()).toHaveCSS('display', 'flex')
      await expectNoSideScroll(page)

      const role = page.getByLabel('Papel de Bruno Lima')
      await expect(role).toHaveValue('staff')
      await expectThumbSized(role, page.getByRole('button', { name: 'Remover Bruno Lima' }), page.getByRole('button', { name: `Reenviar convite para ${invitations[0].email}` }), page.getByRole('button', { name: `Cancelar convite para ${invitations[0].email}` }))

      // The draft change keeps its named button, inside the card.
      await role.selectOption({ label: 'Admin' })
      await expectThumbSized(page.getByRole('button', { name: 'Alterar papel de Bruno Lima' }))
      await expectNoSideScroll(page)
    })

    test('the store picker lists the stores and fits', async ({ page }) => {
      await page.goto('/lojas')
      await expect(page.getByRole('heading', { name: 'Escolha a loja' })).toBeVisible()
      await expect(page.getByRole('button', { name: 'Outra Loja' })).toBeVisible()
      await expectNoSideScroll(page)
      await expectThumbSized(page.getByRole('button', { name: 'Alta RPM' }), page.getByLabel('Nome da loja'), page.getByRole('button', { name: 'Criar loja' }))
    })
  })

  test('the auth screens fit and have thumb-sized controls', async ({ page }) => {
    await page.goto('/')
    await expect(page.getByRole('heading', { name: 'Entrar' })).toBeVisible()
    await expectNoSideScroll(page)
    await expectThumbSized(page.getByLabel('E-mail'), page.getByLabel('Senha'), page.getByRole('button', { name: 'Entrar' }), page.getByRole('link', { name: 'Criar conta' }))

    await page.getByRole('link', { name: 'Criar conta' }).click()
    await expect(page.getByRole('heading', { name: 'Criar conta' })).toBeVisible()
    await expectNoSideScroll(page)
    await expectThumbSized(page.getByLabel('Nome'), page.getByLabel('Senha'), page.getByRole('button', { name: 'Criar conta' }))

    await page.goto('/esqueci-a-senha')
    await expect(page.getByRole('heading', { name: 'Esqueci a senha' })).toBeVisible()
    await expectNoSideScroll(page)
    await expectThumbSized(page.getByLabel('E-mail'), page.getByRole('button', { name: 'Enviar código' }))

    await page.goto('/convite')
    await expect(page.getByRole('heading', { name: 'Abra o convite pelo e-mail' })).toBeVisible()
    await expectNoSideScroll(page)
  })

  test('the menu is a drawer: it opens, traps focus, closes on Escape and after a choice', async ({ page }) => {
    await page.goto('/')
    await page.getByLabel('E-mail').fill('ana@altarpm.com.br')
    await page.getByLabel('Senha').fill('segredo-forte')
    await page.getByRole('button', { name: 'Entrar' }).click()
    await expect(page.getByRole('heading', { name: 'Hoje' })).toBeVisible()
    await expectNoSideScroll(page)

    const menu = page.getByRole('navigation', { name: 'Menu principal' })
    const opener = page.getByRole('button', { name: 'Menu', exact: true })
    await expect(menu).toBeHidden()
    await expect(page.getByRole('status')).toBeVisible()

    await opener.click()
    await expect(menu).toBeVisible()
    await expect(opener).toHaveAttribute('aria-expanded', 'true')
    await expect(menu).toContainText('Alta RPM')
    await expect(menu).toContainText('Ana Souza')
    await expect(menu.getByRole('link', { name: 'Trocar de loja' })).toBeVisible()
    await expect(menu.getByRole('button', { name: 'Sair' })).toBeVisible()
    await expect(menu.getByRole('button', { name: 'Fechar menu' })).toBeFocused()

    // Tab never leaves the drawer.
    for (let step = 0; step < 8; step++) {
      await page.keyboard.press('Tab')
      expect(await menu.evaluate((element) => element.contains(document.activeElement))).toBe(true)
    }
    await page.keyboard.press('Shift+Tab')
    expect(await menu.evaluate((element) => element.contains(document.activeElement))).toBe(true)

    await page.keyboard.press('Escape')
    await expect(menu).toBeHidden()
    await expect(opener).toBeFocused()
    await expect(opener).toHaveAttribute('aria-expanded', 'false')

    // Choosing the page already open closes the drawer too.
    await opener.click()
    await menu.getByRole('link', { name: 'Hoje' }).click()
    await expect(menu).toBeHidden()
    await expectNoSideScroll(page)
  })
})

test.describe('against the real stack', () => {
  test.skip(!realStack, 'set E2E_BASE_URL (and E2E_MAILPIT_URL) to run against `make stack-up`')

  test.beforeEach(() => {
    expect(mailpitUrl, 'E2E_MAILPIT_URL must name Mailpit').not.toBe('')
  })

  test.setTimeout(120_000)

  test('sign-up, the store picker, the menu drawer and Hoje', async ({ page }) => {
    const owner = unique('fone')

    await page.goto('/criar-conta')
    await expectNoSideScroll(page)
    await signUp(page, 'Ana Souza', owner.email)
    await expectNoSideScroll(page)
    await expectThumbSized(page.getByLabel('Nome da loja'), page.getByLabel('Link da loja'), page.getByRole('button', { name: 'Criar loja' }), page.getByRole('button', { name: 'Sair' }))

    await createStore(page, 'Alta RPM', owner.slug)
    await expectNoSideScroll(page)
    await expect(page.getByRole('navigation', { name: 'Menu principal' })).toBeHidden()
    await expect(page.getByRole('banner')).toContainText('Alta RPM')

    // BaseDialog is a full-screen sheet on a phone (Catálogo's own phone layout is step 3.5.4).
    await openMenu(page)
    await page.getByRole('navigation', { name: 'Menu principal' }).getByRole('link', { name: 'Catálogo' }).click()
    await page.getByRole('button', { name: 'Novo item' }).first().click()
    const sheet = page.getByRole('dialog', { name: 'Novo item' })
    await expect(sheet).toBeVisible()
    const box = await sheet.boundingBox()
    expect([box?.x, box?.y, box?.width, box?.height]).toEqual([0, 0, 390, 844])
    await page.keyboard.press('Escape')
    await expect(sheet).toBeHidden()

    // A second store, from the picker: the list of stores and the create form fit.
    await page.goto('/lojas')
    await expect(page.getByRole('heading', { name: 'Escolha a loja' })).toBeVisible()
    await expectNoSideScroll(page)
    await expect(page.getByRole('button', { name: 'Alta RPM' })).toBeVisible()
    await page.getByLabel('Nome da loja').fill('Segunda Loja')
    await page.getByLabel('Link da loja').fill(`${owner.slug}-2`)
    await page.getByRole('button', { name: 'Criar loja' }).click()
    await expect(page.getByRole('heading', { name: 'Hoje' })).toBeVisible()

    // With two stores the drawer offers the switch, and a choice navigates and closes it.
    await openMenu(page)
    const menu = page.getByRole('navigation', { name: 'Menu principal' })
    await expect(menu).toContainText('Segunda Loja')
    await menu.getByRole('link', { name: 'Trocar de loja' }).click()
    await expect(page.getByRole('heading', { name: 'Escolha a loja' })).toBeVisible()
    await expectNoSideScroll(page)
    await page.getByRole('button', { name: 'Alta RPM' }).click()
    await expect(page.getByRole('heading', { name: 'Hoje' })).toBeVisible()
    await expect(menu).toBeHidden()

    await logOut(page)
    await expectNoSideScroll(page)
  })

  test('Equipe: the invite form, the cards, a role change and a cancelled invitation', async ({ page, request, browser }) => {
    const owner = unique('fonequipe')
    const invitee = unique('fonegente')

    await signUp(page, 'Carla Dias', owner.email)
    await createStore(page, 'Equipe Fone', owner.slug)
    await openMenu(page)
    await page.getByRole('navigation', { name: 'Menu principal' }).getByRole('link', { name: 'Equipe' }).click()
    await expect(page.getByRole('heading', { name: 'Equipe', exact: true })).toBeVisible()
    await expect(page.getByRole('navigation', { name: 'Menu principal' })).toBeHidden()
    await expect(page.getByText('Carla Dias (você)')).toBeVisible()
    await expectNoSideScroll(page)

    const form = page.getByRole('form', { name: 'Convidar para a equipe' })
    await expectThumbSized(form.getByLabel('E-mail'), form.getByLabel('Papel'), form.getByRole('button', { name: 'Enviar convite' }))
    await form.getByLabel('E-mail').fill(invitee.email)
    await form.getByRole('button', { name: 'Enviar convite' }).click()
    await expect(page.getByText(`Convite enviado para ${invitee.email}.`)).toBeVisible()
    await expect(page.getByText(invitee.email, { exact: true })).toBeVisible()
    await expectNoSideScroll(page)
    // Every action keeps its accessible name in the pending invitation's card.
    await expectThumbSized(page.getByRole('button', { name: `Reenviar convite para ${invitee.email}` }), page.getByRole('button', { name: `Cancelar convite para ${invitee.email}` }))

    // The invited person accepts on a phone too.
    const link = /http\S*\/convite#token=[\w-]+/.exec(await latestEmail(request, invitee.email))?.[0]
    expect(link, 'the invitation link in the email').toBeDefined()
    const url = new URL(link ?? '')
    const context = await browser.newContext({ ...test.info().project.use, baseURL: process.env.E2E_BASE_URL })
    try {
      const guest = await context.newPage()
      await guest.goto(new URL(url.pathname + url.hash, process.env.E2E_BASE_URL).toString())
      await expect(guest.getByRole('heading', { name: 'Entrar na equipe da loja Equipe Fone' })).toBeVisible()
      await expectNoSideScroll(guest)
      await expectThumbSized(guest.getByLabel('Nome'), guest.getByLabel('Senha'), guest.getByRole('button', { name: 'Aceitar convite' }))
      await guest.getByLabel('Nome').fill('Davi Rocha')
      await guest.getByLabel('Senha').fill(password)
      await guest.getByRole('button', { name: 'Aceitar convite' }).click()
      await expect(guest).toHaveURL(/\/hoje$/)
      await expectNoSideScroll(guest)
    } finally {
      await context.close()
    }

    await page.reload()
    const role = page.getByLabel('Papel de Davi Rocha')
    await expect(role).toHaveValue('staff')
    await expectNoSideScroll(page)
    await expectThumbSized(role, page.getByRole('button', { name: 'Remover Davi Rocha' }))
    await role.selectOption({ label: 'Admin' })
    await page.getByRole('button', { name: 'Alterar papel de Davi Rocha' }).click()
    await expect(page.getByText('Davi Rocha agora é Admin.')).toBeVisible()
    await expect(role).toHaveValue('admin')

    // The question that replaces the buttons fits too.
    await page.getByRole('button', { name: 'Remover Davi Rocha' }).click()
    await expect(page.getByText('Remover Davi Rocha da equipe?')).toBeVisible()
    await expectNoSideScroll(page)
    await page.getByRole('button', { name: 'Cancelar', exact: true }).click()
    await expect(role).toBeVisible()

    const other = unique('fonecancel')
    await form.getByLabel('E-mail').fill(other.email)
    await form.getByRole('button', { name: 'Enviar convite' }).click()
    await expect(page.getByText(`Convite enviado para ${other.email}.`)).toBeVisible()
    await page.getByRole('button', { name: `Cancelar convite para ${other.email}` }).click()
    await expect(page.getByText(`Convite para ${other.email} cancelado.`)).toBeVisible()
    await expectNoSideScroll(page)
  })
})
