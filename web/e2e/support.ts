import { expect, type APIRequestContext, type Page } from '@playwright/test'

// Helpers the real-stack flows share: sign-up, the first store, log-out and
// the Mailpit inbox. See admin.spec.ts for how to run them.
export const mailpitUrl = process.env.E2E_MAILPIT_URL ?? ''
export const password = 'segredo-forte-1'

/** unique makes an email and a store link that no earlier run or parallel test used. */
export function unique(label: string): { email: string; slug: string } {
  const id = `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 8)}`
  return { email: `${label}-${id}@e2e.test`, slug: `${label}-${id}` }
}

export async function signUp(page: Page, name: string, email: string): Promise<void> {
  await page.goto('/criar-conta')
  await page.getByLabel('Nome', { exact: true }).fill(name)
  await page.getByLabel('E-mail').fill(email)
  await page.getByLabel('Senha').fill(password)
  await page.getByRole('button', { name: 'Criar conta' }).click()
  await expect(page.getByRole('heading', { name: 'Crie sua loja' })).toBeVisible()
}

export async function createStore(page: Page, name: string, slug: string): Promise<void> {
  await page.getByLabel('Nome da loja').fill(name)
  await page.getByLabel('Link da loja').fill(slug)
  await page.getByRole('button', { name: 'Criar loja' }).click()
  await expect(page).toHaveURL(/\/hoje$/)
  await expect(page.getByRole('heading', { name: 'Hoje' })).toBeVisible()
}

/**
 * openMenu opens the menu drawer when the page is at phone width, where the
 * menu hides behind the "Menu" button, and does nothing at desktop width.
 */
export async function openMenu(page: Page): Promise<void> {
  const menu = page.getByRole('navigation', { name: 'Menu principal' })
  const button = page.getByRole('button', { name: 'Menu', exact: true })
  if (!(await menu.isVisible()) && (await button.isVisible())) await button.click()
  await expect(menu).toBeVisible()
}

export async function logOut(page: Page): Promise<void> {
  await openMenu(page)
  await page.getByRole('navigation', { name: 'Menu principal' }).getByRole('button', { name: 'Sair' }).click()
  await expect(page.getByRole('heading', { name: 'Entrar' })).toBeVisible()
}

/**
 * latestEmail polls Mailpit until `count` messages for `to` have arrived
 * and returns the text body of the newest one. Mailpit lists newest first.
 */
export async function latestEmail(request: APIRequestContext, to: string, count = 1): Promise<string> {
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

/** Api is a signed-in session for seeding data through the API where the UI path is long and covered elsewhere. */
export interface Api {
  send(method: 'GET' | 'POST' | 'PUT', path: string, data?: unknown): Promise<unknown>
}

/**
 * apiSession logs `email` in through the API and returns a caller for it.
 * Node cannot resolve *.localhost the way Chromium does, so it calls the
 * stack by address and keeps the admin Host header nginx routes on. A new
 * login has the account's only store selected, so call it after the store
 * exists.
 */
export async function apiSession(request: APIRequestContext, email: string): Promise<Api> {
  const admin = new URL(process.env.E2E_BASE_URL ?? '')
  const base = `http://127.0.0.1:${admin.port}`
  const json = { 'content-type': 'application/json', host: admin.host }
  const login = await request.post(`${base}/auth/login`, { headers: json, data: { email, password } })
  expect(login.status(), 'API login').toBe(200)
  const headers = { ...json, authorization: `Bearer ${((await login.json()) as { accessToken: string }).accessToken}` }
  return {
    async send(method, path, data) {
      const response = await request.fetch(`${base}${path}`, { method, headers, data })
      expect(response.status(), `${method} ${path}: ${await response.text()}`).toBeLessThan(300)
      return response.status() === 204 ? null : response.json()
    },
  }
}

/** Catalog names what seedCatalog made: two items and a kit holding both. */
export interface Catalog {
  kit: string
  first: string
  second: string
}

/**
 * seedCatalog makes a supplier, two made-to-order items with a cost each and
 * a kit holding both, through the API. Catálogo has its own UI flow
 * (catalogo.spec.ts); the order flows only need something to sell.
 */
export async function seedCatalog(api: Api): Promise<Catalog> {
  const supplier = (await api.send('POST', '/api/v1/admin/suppliers', { name: 'Escapes do Sul', whatsappPhone: '5511987654321' })) as { id: string }
  const item = async (name: string, priceCents: number, leadDays: number, costCents: number): Promise<string> => {
    const created = (await api.send('POST', '/api/v1/admin/catalog/items', {
      name,
      description: '',
      categoryId: null,
      sourcingMode: 'to_order',
      priceCents,
      mainSupplier: { supplierId: supplier.id, leadDays, costCents },
      attributeValueIds: [],
      options: [],
      published: true,
    })) as { id: string }
    return created.id
  }
  const first = await item('Escapamento esportivo', 45000, 5, 28000)
  const second = await item('Filtro de ar', 12000, 3, 7000)
  await api.send('POST', '/api/v1/admin/catalog/kits', {
    name: 'Kit esportivo',
    description: '',
    priceCents: 52000,
    swapDiscount: 'keep',
    assemblyDays: null,
    safetyMarginDays: null,
    items: [
      { itemId: first, quantity: 1, swappable: false, alternativeItemIds: [] },
      { itemId: second, quantity: 1, swappable: false, alternativeItemIds: [] },
    ],
    attributeValueIds: [],
    published: true,
  })
  return { kit: 'Kit esportivo', first: 'Escapamento esportivo', second: 'Filtro de ar' }
}
