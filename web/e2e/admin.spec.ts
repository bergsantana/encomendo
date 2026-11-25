import { expect, test } from '@playwright/test'

// These run against the built app on `vite preview`, which has no API, so
// /auth is answered here: no session cookie, and one login. The real login
// flow against the docker compose stack belongs to m2-integrate, once the API
// serves /auth (docs/parallel-sessions.md). The routes still let a real
// browser run the built app, Web Lock included, which jsdom cannot.
test.beforeEach(async ({ page }) => {
  await page.route('**/auth/refresh', (route) =>
    route.fulfill({
      status: 401,
      contentType: 'application/problem+json',
      body: JSON.stringify({ type: 'about:blank', title: 'Sessão expirada', status: 401, code: 'unauthenticated' }),
    }),
  )
})

test('a signed-out visit opens the login form', async ({ page }) => {
  await page.goto('/')

  await expect(page).toHaveURL(/\/entrar\?redirect=/)
  await expect(page.getByRole('heading', { name: 'Entrar' })).toBeVisible()
  await expect(page.getByLabel('E-mail')).toBeVisible()
  await expect(page.getByLabel('Senha')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Entrar' })).toBeVisible()
})

test('logging in opens Hoje with the main menu', async ({ page }) => {
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
        memberships: [{ tenant: store, role: 'owner' }],
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
  // Without a backend the status line must still render, in pt-BR.
  await expect(page.getByRole('status')).toBeVisible()
})
