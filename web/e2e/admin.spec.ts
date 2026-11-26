import { expect, test, type APIRequestContext, type Page } from '@playwright/test'

// The flows run in a real browser against the full stack (`make stack-up`):
// the admin app behind nginx, the API on the same origin and Mailpit catching
// the emails. Set E2E_BASE_URL (the admin app, e.g. http://admin.localhost:8110)
// and E2E_MAILPIT_URL (e.g. http://localhost:8055) to run them. Without
// E2E_BASE_URL the suite runs against `vite preview`, which has no API, so only
// the smoke tests at the top run and the real-stack tests skip.
//
// One run signs up three accounts from one address, and the API allows 10
// sign-ups an hour per IP (ADR 0021). To run more than three times in an
// hour against the same stack, clear the counts first:
//   docker exec <redis container> sh -c "redis-cli --scan --pattern 'g:rl:*' | xargs -r redis-cli del"
// With E2E_BASE_URL set and E2E_MAILPIT_URL missing, the real-stack tests
// fail on purpose rather than skip.
const realStack = process.env.E2E_BASE_URL !== undefined && process.env.E2E_BASE_URL !== ''
const mailpitUrl = process.env.E2E_MAILPIT_URL ?? ''
const password = 'segredo-forte-1'

test.describe('without an API', () => {
  test.skip(realStack, 'runs only against `vite preview`, which has no API')

  // /auth/refresh is the one call the app makes before it knows who is
  // there. The route lets a real browser run the built app, Web Lock
  // included, which jsdom cannot.
  test('a signed-out visit opens the login form', async ({ page }) => {
    await page.route('**/auth/refresh', (route) =>
      route.fulfill({
        status: 401,
        contentType: 'application/problem+json',
        body: JSON.stringify({ type: 'about:blank', title: 'Sessão expirada', status: 401, code: 'unauthenticated' }),
      }),
    )

    await page.goto('/')

    await expect(page).toHaveURL(/\/entrar\?redirect=/)
    await expect(page.getByRole('heading', { name: 'Entrar' })).toBeVisible()
    await expect(page.getByLabel('E-mail')).toBeVisible()
    await expect(page.getByLabel('Senha')).toBeVisible()
    await expect(page.getByRole('button', { name: 'Entrar' })).toBeVisible()
  })

  // Loads the lazy Hoje screen, the menu and the session store from the
  // built bundle, which the real-stack tests do not run in CI yet.
  test('logging in opens Hoje with the main menu', async ({ page }) => {
    await page.route('**/auth/refresh', (route) =>
      route.fulfill({
        status: 401,
        contentType: 'application/problem+json',
        body: JSON.stringify({ type: 'about:blank', title: 'Sessão expirada', status: 401, code: 'unauthenticated' }),
      }),
    )
    const store = { id: '0190f3c4-5e1a-7b2c-8d3e-4f5a6b7c8d00', name: 'Alta RPM', slug: 'altarpm' }
    await page.route('**/auth/login', (route) =>
      route.fulfill({
        json: {
          accessToken: 'e2e-access',
          tokenType: 'Bearer',
          expiresIn: 900,
          user: { id: '0190f3c4-5e1a-7b2c-8d3e-4f5a6b7c8d9e', name: 'Ana Souza', email: 'ana@altarpm.com.br' },
          tenant: store,
          role: 'owner',
          permissions: ['order:read', 'order:write'],
          memberships: [{ tenant: store, role: 'owner', status: 'active' }],
        },
      }),
    )

    await page.goto('/')
    await page.getByLabel('E-mail').fill('ana@altarpm.com.br')
    await page.getByLabel('Senha').fill('segredo-forte')
    await page.getByRole('button', { name: 'Entrar' }).click()

    await expect(page).toHaveURL(/\/hoje$/)
    await expect(page.getByRole('heading', { name: 'Hoje' })).toBeVisible()
    const menu = page.getByRole('navigation', { name: 'Menu principal' })
    await expect(menu).toContainText('Hoje')
    await expect(menu).toContainText('Ana Souza')
    await expect(page.getByRole('status')).toBeVisible()
  })
})

test.describe('against the real stack', () => {
  test.skip(!realStack, 'set E2E_BASE_URL (and E2E_MAILPIT_URL) to run against `make stack-up`')

  /** unique makes an email and a store link that no earlier run or parallel test used. */
  function unique(label: string): { email: string; slug: string } {
    const id = `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 8)}`
    return { email: `${label}-${id}@e2e.test`, slug: `${label}-${id}` }
  }

  async function signUp(page: Page, name: string, email: string): Promise<void> {
    await page.goto('/criar-conta')
    await page.getByLabel('Nome', { exact: true }).fill(name)
    await page.getByLabel('E-mail').fill(email)
    await page.getByLabel('Senha').fill(password)
    await page.getByRole('button', { name: 'Criar conta' }).click()
    await expect(page.getByRole('heading', { name: 'Crie sua loja' })).toBeVisible()
  }

  async function createStore(page: Page, name: string, slug: string): Promise<void> {
    await page.getByLabel('Nome da loja').fill(name)
    await page.getByLabel('Link da loja').fill(slug)
    await page.getByRole('button', { name: 'Criar loja' }).click()
    await expect(page).toHaveURL(/\/hoje$/)
    await expect(page.getByRole('heading', { name: 'Hoje' })).toBeVisible()
  }

  async function logOut(page: Page): Promise<void> {
    await page.getByRole('navigation', { name: 'Menu principal' }).getByRole('button', { name: 'Sair' }).click()
    await expect(page.getByRole('heading', { name: 'Entrar' })).toBeVisible()
  }

  /**
   * latestEmail polls Mailpit until `count` messages for `to` have arrived
   * and returns the text body of the newest one. Mailpit lists newest first.
   */
  async function latestEmail(request: APIRequestContext, to: string, count = 1): Promise<string> {
    let text = ''
    await expect
      .poll(
        async () => {
          const found = await request.get(`${mailpitUrl}/api/v1/search`, { params: { query: `to:${to}` } })
          expect(found.ok(), `Mailpit at ${mailpitUrl} answered ${found.status()}`).toBe(true)
          const messages = ((await found.json()) as { messages?: { ID: string }[] }).messages ?? []
          if (messages.length < count) return false
          const message = await request.get(`${mailpitUrl}/api/v1/message/${messages[0].ID}`)
          text = ((await message.json()) as { Text: string }).Text
          return true
        },
        // Reset codes go out off the request path: give slow CI time.
        { message: `${count} email(s) to ${to} in Mailpit`, timeout: 15_000 },
      )
      .toBe(true)
    return text
  }

  test.beforeEach(() => {
    expect(mailpitUrl, 'E2E_MAILPIT_URL must name Mailpit').not.toBe('')
  })

  test('sign-up, stores, Hoje, then log out and log in again', async ({ page }) => {
    const owner = unique('dono')

    await signUp(page, 'Ana Souza', owner.email)
    // A reload restores the store-less session from the refresh cookie.
    await page.reload()
    await expect(page.getByRole('heading', { name: 'Crie sua loja' })).toBeVisible()

    // The API's own refusal of a link shows next to the field.
    await page.getByLabel('Nome da loja').fill('Alta RPM')
    await page.getByLabel('Link da loja').fill('a_')
    await page.getByRole('button', { name: 'Criar loja' }).click()
    await expect(page.getByText('Confira os campos destacados.')).toBeVisible()
    await expect(page.getByText('Use de 3 a 40 letras minúsculas, números e hífens, sem hífen no começo ou no fim.')).toBeVisible()

    await createStore(page, 'Alta RPM', owner.slug)
    const menu = page.getByRole('navigation', { name: 'Menu principal' })
    await expect(menu).toContainText('Alta RPM')
    await expect(menu).toContainText('Ana Souza')
    await expect(menu.getByRole('link', { name: 'Trocar de loja' })).toHaveCount(0)

    await logOut(page)
    await page.getByLabel('E-mail').fill(owner.email)
    await page.getByLabel('Senha').fill('outra-senha-errada')
    await page.getByRole('button', { name: 'Entrar' }).click()
    await expect(page.getByText('E-mail ou senha incorretos.')).toBeVisible()

    await page.getByLabel('Senha').fill(password)
    await page.getByRole('button', { name: 'Entrar' }).click()
    await expect(page).toHaveURL(/\/hoje$/)
    await expect(menu).toContainText('Alta RPM')

    // A reload inside the store restores the session too.
    await page.reload()
    await expect(page.getByRole('heading', { name: 'Hoje' })).toBeVisible()

    // A second store: its link must be free, and the menu then offers the switch.
    await page.goto('/lojas')
    await expect(page.getByRole('heading', { name: 'Escolha a loja' })).toBeVisible()
    await expect(page.getByText(`${owner.slug}.`)).toBeVisible()
    await page.getByLabel('Nome da loja').fill('Segunda Loja')
    await page.getByLabel('Link da loja').fill(owner.slug)
    await page.getByRole('button', { name: 'Criar loja' }).click()
    await expect(page.getByText('Este link já está em uso. Escolha outro.')).toBeVisible()

    await page.getByLabel('Link da loja').fill(`${owner.slug}-2`)
    await page.getByRole('button', { name: 'Criar loja' }).click()
    await expect(page).toHaveURL(/\/hoje$/)
    await expect(menu).toContainText('Segunda Loja')

    await menu.getByRole('link', { name: 'Trocar de loja' }).click()
    await page.getByRole('button', { name: 'Alta RPM' }).click()
    await expect(page).toHaveURL(/\/hoje$/)
    await expect(menu).toContainText('Alta RPM')

    // A taken email at sign-up is covered by the unit tests: here it would
    // spend a fourth sign-up of the per-IP limit.
  })

  test('Esqueci a senha: the code from the email sets a new password', async ({ page, request }) => {
    const owner = unique('reset')
    await signUp(page, 'Bia Lima', owner.email)
    await createStore(page, 'Bia Peças', owner.slug)
    await logOut(page)

    await page.getByRole('link', { name: 'Esqueci a senha' }).click()
    // The login form stays on screen while the next screen loads: wait for it before typing.
    await expect(page.getByRole('heading', { name: 'Esqueci a senha' })).toBeVisible()
    await page.getByLabel('E-mail').fill(owner.email)
    await page.getByRole('button', { name: 'Enviar código' }).click()
    await expect(page.getByText(`Se houver uma conta com ${owner.email}, enviamos um código de 6 dígitos.`)).toBeVisible()

    const body = await latestEmail(request, owner.email)
    const code = /^\s+(\d{6})\s*$/m.exec(body)?.[1]
    expect(code, 'a 6-digit code on its own line in the email').toBeDefined()

    await page.getByLabel('Código').fill('000000')
    await page.getByLabel('Nova senha').fill('nova-senha-forte')
    await page.getByRole('button', { name: 'Alterar senha' }).click()
    await expect(page.getByText('Código errado ou vencido. Confira o e-mail ou peça um novo código.')).toBeVisible()

    await page.getByLabel('Código').fill(code ?? '')
    await page.getByRole('button', { name: 'Alterar senha' }).click()
    await expect(page.getByText('Senha alterada. Entre com a nova senha.')).toBeVisible()

    await page.getByLabel('E-mail').fill(owner.email)
    await page.getByLabel('Senha').fill('nova-senha-forte')
    await page.getByRole('button', { name: 'Entrar' }).click()
    await expect(page).toHaveURL(/\/hoje$/)
  })

  test('Equipe: invite, accept, change the role, remove, then invite the same person again', async ({ page, request, browser }) => {
    const owner = unique('equipe')
    const invitee = unique('convidado')
    const newBrowser = () => browser.newContext({ baseURL: process.env.E2E_BASE_URL, locale: 'pt-BR' })
    const sendInvitation = async (email: string, role?: string): Promise<void> => {
      await page.getByRole('form', { name: 'Convidar para a equipe' }).getByLabel('E-mail').fill(email)
      if (role !== undefined) await page.getByLabel('Papel', { exact: true }).selectOption({ label: role })
      await page.getByRole('button', { name: 'Enviar convite', exact: true }).click()
      await expect(page.getByText(`Convite enviado para ${email}.`)).toBeVisible()
    }
    // The email's link points at the API's configured admin origin; rebuild
    // it on E2E_BASE_URL so the test never leaves the stack under test.
    const invitationLink = async (email: string, count = 1): Promise<string> => {
      const link = /http\S*\/convite#token=[\w-]+/.exec(await latestEmail(request, email, count))?.[0]
      expect(link, 'the invitation link in the email').toBeDefined()
      const url = new URL(link ?? '')
      return new URL(url.pathname + url.hash, process.env.E2E_BASE_URL).toString()
    }

    await signUp(page, 'Carla Dias', owner.email)
    await createStore(page, 'Equipe Teste', owner.slug)
    await page.getByRole('navigation', { name: 'Menu principal' }).getByRole('link', { name: 'Equipe' }).click()
    await expect(page.getByRole('heading', { name: 'Equipe', exact: true })).toBeVisible()
    await expect(page.getByRole('row', { name: /Carla Dias \(você\)/ })).toContainText('Dono')

    await sendInvitation(invitee.email)
    await expect(page.getByRole('row', { name: new RegExp(invitee.email) })).toBeVisible()

    // The invited person has no session: a second browser, as in real life.
    const firstLink = await invitationLink(invitee.email)
    const newcomer = await newBrowser()
    try {
      const guest = await newcomer.newPage()
      await guest.goto(firstLink)
      await expect(guest.getByRole('heading', { name: 'Entrar na equipe da loja Equipe Teste' })).toBeVisible()
      await expect(guest.getByText(`Convite para ${invitee.email}. Crie sua conta para aceitar.`)).toBeVisible()
      await guest.getByLabel('Nome').fill('Davi Rocha')
      await guest.getByLabel('Senha').fill(password)
      await guest.getByRole('button', { name: 'Aceitar convite' }).click()
      await expect(guest).toHaveURL(/\/hoje$/)
      const guestMenu = guest.getByRole('navigation', { name: 'Menu principal' })
      await expect(guestMenu).toContainText('Davi Rocha')
      // Funcionário cannot manage the team, so the entry is hidden.
      await expect(guestMenu.getByRole('link', { name: 'Equipe' })).toHaveCount(0)

      // The link works once.
      await guest.goto(firstLink)
      await expect(guest.getByRole('heading', { name: 'Convite indisponível' })).toBeVisible()
      await expect(guest.getByText('Este convite não vale mais. Peça um novo a quem convidou você.')).toBeVisible()
    } finally {
      await newcomer.close()
    }

    // The owner reads the team again: the invitation became a member.
    await page.reload()
    const member = page.getByRole('row', { name: /Davi Rocha/ })
    await expect(member).toContainText(invitee.email)
    // The invitation's role (Funcionário, the default) is the member's.
    await expect(member.getByLabel('Papel de Davi Rocha')).toHaveValue('staff')

    await member.getByLabel('Papel de Davi Rocha').selectOption({ label: 'Admin' })
    await member.getByRole('button', { name: 'Alterar papel de Davi Rocha' }).click()
    await expect(page.getByText('Davi Rocha agora é Admin.')).toBeVisible()
    await expect(member.getByLabel('Papel de Davi Rocha')).toHaveValue('admin')

    await page.getByRole('form', { name: 'Convidar para a equipe' }).getByLabel('E-mail').fill(invitee.email)
    await page.getByRole('button', { name: 'Enviar convite', exact: true }).click()
    await expect(page.getByText('Esta pessoa já faz parte da equipe.')).toBeVisible()

    await member.getByRole('button', { name: 'Remover Davi Rocha' }).click()
    await expect(page.getByText('Remover Davi Rocha da equipe?')).toBeVisible()
    await member.getByRole('button', { name: 'Confirmar remoção' }).click()
    await expect(page.getByText('Davi Rocha saiu da equipe.')).toBeVisible()
    await expect(page.getByRole('row', { name: /Davi Rocha/ })).toHaveCount(0)

    // Davi now has an account: the new invitation asks for the password only.
    await sendInvitation(invitee.email, 'Visualizador')
    const secondLink = await invitationLink(invitee.email, 2)
    expect(secondLink).not.toBe(firstLink)
    const returning = await newBrowser()
    try {
      const guest = await returning.newPage()
      await guest.goto(secondLink)
      await expect(guest.getByText(`Você já tem uma conta com ${invitee.email}. Entre com a sua senha para aceitar.`)).toBeVisible()
      await guest.getByLabel('Senha').fill('senha-errada-123')
      await guest.getByRole('button', { name: 'Aceitar convite' }).click()
      await expect(guest.getByText('Senha incorreta.')).toBeVisible()

      await guest.getByLabel('Senha').fill(password)
      await guest.getByRole('button', { name: 'Aceitar convite' }).click()
      await expect(guest).toHaveURL(/\/hoje$/)
      await expect(guest.getByRole('navigation', { name: 'Menu principal' })).toContainText('Equipe Teste')
    } finally {
      await returning.close()
    }
    // The second invitation's role (Visualizador) is the member's now.
    await page.reload()
    await expect(page.getByRole('row', { name: /Davi Rocha/ }).getByLabel('Papel de Davi Rocha')).toHaveValue('viewer')

    // Another invitation, cancelled before it is used.
    const other = unique('cancelado')
    await sendInvitation(other.email)
    await page.getByRole('button', { name: `Cancelar convite para ${other.email}` }).click()
    await expect(page.getByText(`Convite para ${other.email} cancelado.`)).toBeVisible()
    await expect(page.getByRole('row', { name: new RegExp(other.email) })).toHaveCount(0)
  })
})
