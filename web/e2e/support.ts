import { expect, type APIRequestContext, type Locator, type Page } from '@playwright/test'

// Helpers the real-stack flows share: sign-up, the first store, log-out and
// the Mailpit inbox. See admin.spec.ts for how to run them.
export const mailpitUrl = process.env.E2E_MAILPIT_URL ?? ''
export const password = 'segredo-forte-1'

/** unique makes an email and a store link that no earlier run or parallel test used. */
export function unique(label: string): { email: string; slug: string } {
  const id = `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 8)}`
  return { email: `${label}-${id}@e2e.test`, slug: `${label}-${id}` }
}

/**
 * ownClientAddress gives the sign-up call of `page` a client address of its
 * own, so the suite's sign-ups do not share the API's 10-an-hour-per-IP limit
 * (ADR 0021) from the one address of the machine that runs it. The local
 * compose stack trusts the private ranges, so a client on its published port
 * picks its own rate-limit key (deploy/docker-compose.yml, accepted for local
 * stacks only). Only the sign-up call carries the header, so a cross-origin
 * request, such as a photo upload, is never given an extra header to preflight.
 */
async function ownClientAddress(page: Page): Promise<void> {
  // 198.18.0.0/15 is the benchmarking range: public as far as the API is concerned, never a real client.
  const address = `198.18.${Math.floor(Math.random() * 256)}.${1 + Math.floor(Math.random() * 254)}`
  await page.route('**/auth/signup', (route) => route.continue({ headers: { ...route.request().headers(), 'x-forwarded-for': address } }))
}

export async function signUp(page: Page, name: string, email: string): Promise<void> {
  await ownClientAddress(page)
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
  /** send calls the API and returns the JSON answer as T, which the caller states; a status of 300 or more fails the test. */
  send<T = unknown>(method: 'GET' | 'POST', path: string, data?: unknown, headers?: Record<string, string>): Promise<T>
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
    async send<T>(method: 'GET' | 'POST', path: string, data?: unknown, extra?: Record<string, string>): Promise<T> {
      const response = await request.fetch(`${base}${path}`, { method, headers: { ...headers, ...extra }, data })
      expect(response.status(), `${method} ${path}: ${await response.text()}`).toBeLessThan(300)
      return (await response.json()) as T
    },
  }
}

/** Catalog names and identifies what seedCatalog made: two items and a kit holding both. */
export interface Catalog {
  kit: string
  kitId: string
  first: string
  second: string
}

/**
 * seedCatalog makes a supplier, two made-to-order items with a cost each and
 * a kit holding both, through the API. Catálogo has its own UI flow
 * (catalogo.spec.ts); the order flows only need something to sell.
 */
export async function seedCatalog(api: Api): Promise<Catalog> {
  const supplier = await api.send<{ id: string }>('POST', '/api/v1/admin/suppliers', { name: 'Escapes do Sul', whatsappPhone: '5511987654321' })
  const item = async (name: string, priceCents: number, leadDays: number, costCents: number): Promise<string> => {
    const created = await api.send<{ id: string }>('POST', '/api/v1/admin/catalog/items', {
      name,
      description: '',
      categoryId: null,
      sourcingMode: 'to_order',
      priceCents,
      mainSupplier: { supplierId: supplier.id, leadDays, costCents },
      attributeValueIds: [],
      options: [],
      published: true,
    })
    return created.id
  }
  const first = await item('Escapamento esportivo', 45000, 5, 28000)
  const second = await item('Filtro de ar', 12000, 3, 7000)
  const kit = await api.send<{ id: string }>('POST', '/api/v1/admin/catalog/kits', {
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
  return { kit: 'Kit esportivo', kitId: kit.id, first: 'Escapamento esportivo', second: 'Filtro de ar' }
}

/**
 * watchWhatsapp stubs window.open before any page script runs, so a wa.me tab
 * never really opens, and returns the links the app tried to open and every
 * request the page made. The stub lives on the document, so a flow that reads
 * `opened` must stay inside the SPA (no reload or full navigation before it).
 */
export async function watchWhatsapp(page: Page): Promise<{ opened: () => Promise<string[]>; requests: string[] }> {
  const requests: string[] = []
  page.on('request', (request) => requests.push(request.url()))
  await page.addInitScript(() => {
    const opened: string[] = []
    Object.assign(window, { __opened: opened })
    window.open = (url?: string | URL) => {
      opened.push(String(url))
      return null
    }
  })
  return { requests, opened: () => page.evaluate(() => (window as unknown as { __opened: string[] }).__opened) }
}

/** expectNoWhatsappRequests fails when the browser called WhatsApp itself: the app only hands over a link. */
export function expectNoWhatsappRequests(requests: string[]): void {
  expect(requests.filter((url) => /(^|\.)(wa\.me|whatsapp\.com|whatsapp\.net)(\/|:|$)/.test(new URL(url).host + '/')), 'requests to WhatsApp').toEqual([])
  expect(requests.length, 'the browser made requests').toBeGreaterThan(0)
}

/** itemPhoto is a 1 x 1 JPEG, the smallest photo the API accepts, for the file inputs of Receber itens. */
export const itemPhoto = {
  name: 'item.jpg',
  mimeType: 'image/jpeg',
  buffer: Buffer.from(
    '/9j/4AAQSkZJRgABAQEASABIAAD/2wBDAP//////////////////////////////////////////////////////////////////////////////////////wgALCAABAAEBAREA/8QAFBABAAAAAAAAAAAAAAAAAAAAAP/aAAgBAQABPxA=',
    'base64',
  ),
}

/** Bought names the supplier and the item seedBoughtItem made. */
export interface Bought {
  supplier: string
  supplierPhone: string
  item: string
  itemId: string
}

/**
 * seedBoughtItem makes a supplier and one made-to-order item priced at
 * R$ 300,00 (cost R$ 180,00, 4 days), the item Compras buys. Catálogo and
 * Fornecedores have their own UI flows; Compras only needs something to buy.
 */
export async function seedBoughtItem(api: Api): Promise<Bought> {
  const supplier = await api.send<{ id: string }>('POST', '/api/v1/admin/suppliers', { name: 'Couros Silva', whatsappPhone: '5511987654321' })
  const created = await api.send<{ id: string }>('POST', '/api/v1/admin/catalog/items', {
    name: 'Banco de couro',
    description: '',
    categoryId: null,
    sourcingMode: 'to_order',
    priceCents: 30000,
    mainSupplier: { supplierId: supplier.id, leadDays: 4, costCents: 18000 },
    attributeValueIds: [],
    options: [],
    published: true,
  })
  return { supplier: 'Couros Silva', supplierPhone: '5511987654321', item: 'Banco de couro', itemId: created.id }
}

/** PlacedOrder is an order placeOrder made. */
export interface PlacedOrder {
  id: string
  number: string
}

/**
 * placeOrder sells one `itemId` to a new customer through the API and, with
 * `deposit`, records the deposit by hand, which releases the order so its
 * bought items reach "A comprar". The order screen and Novo pedido have their
 * own UI flows (pedidos.spec.ts); the purchasing flows only need orders.
 */
export async function placeOrder(api: Api, itemId: string, key: string, deposit: boolean): Promise<PlacedOrder> {
  const created = await api.send<{ order: { id: string; number: string; version: number; depositOwed: { cents: number } } }>(
    'POST',
    '/api/v1/admin/orders',
    { newCustomer: { name: 'Rafael Lima', phone: '5592991234567' }, lines: [{ itemId, quantity: 1 }], terms: { handover: 'pickup', freightCents: 0 } },
    { 'idempotency-key': `${key}-order` },
  )
  const { order } = created
  if (deposit) {
    await api.send('POST', `/api/v1/admin/orders/${order.id}/payments`, { kind: 'deposit', amountCents: order.depositOwed.cents, method: 'pix', version: order.version }, { 'idempotency-key': `${key}-deposit` })
  }
  return { id: order.id, number: order.number }
}

/**
 * expectThumbSized checks the controls a thumb must hit are at least `min` px
 * tall (48 for the management screens, 56 for Receber itens). A checkbox or
 * radio is hit through its label, which is what is measured.
 */
export async function expectThumbSized(min: number, ...controls: Locator[]): Promise<void> {
  for (const control of controls) {
    const height = await control.evaluate((element) => {
      const boxed = element instanceof HTMLInputElement && (element.type === 'checkbox' || element.type === 'radio')
      const target = (boxed ? element.closest('label') : null) ?? element
      return { height: target.getBoundingClientRect().height, html: element.outerHTML.slice(0, 100) }
    })
    expect(height.height, height.html).toBeGreaterThanOrEqual(min)
  }
}

/** expectNoSideScroll fails when the page is wider than the screen: a phone must never scroll sideways. */
export async function expectNoSideScroll(page: Page): Promise<void> {
  const widths = await page.evaluate(() => ({ scroll: document.documentElement.scrollWidth, inner: window.innerWidth }))
  expect(widths.scroll, `page is ${widths.scroll} px wide on a ${widths.inner} px screen`).toBeLessThanOrEqual(widths.inner)
}

/** expectSheetFits checks the open sheet fills the 390 x 844 screen and nothing in it spills sideways. */
export async function expectSheetFits(page: Page, name: string | RegExp): Promise<Locator> {
  const sheet = page.getByRole('dialog', typeof name === 'string' ? { name, exact: true } : { name })
  await expect(sheet).toBeVisible()
  const box = await sheet.boundingBox()
  expect([box?.x, box?.y, box?.width, box?.height]).toEqual([0, 0, 390, 844])
  await expectNoSideScroll(page)
  const inner = await sheet.evaluate((element) => {
    const body = element.querySelector('.enc-dialog__body')
    return body === null ? 0 : body.scrollWidth - body.clientWidth
  })
  expect(inner, 'the sheet scrolls sideways').toBeLessThanOrEqual(0)
  return sheet
}
